import { aws_ec2 as ec2, aws_eks as eks, aws_iam as iam, Stack, StackProps, STACK_RESOURCE_LIMIT_CONTEXT, Tags } from 'aws-cdk-lib'
import { SecurityGroup } from 'aws-cdk-lib/aws-ec2'
import { Construct } from 'constructs'
import { AWSEBSCSIDriver } from './addons/aws-ebs-csi-driver'
import { AWSLoadBalancerController } from './addons/aws-lbc'
import { FluxV2 } from './addons/fluxv2'
import { Karpenter } from './addons/karpenter'
import { KIT } from './addons/kit'

export class KITInfrastructure extends Stack {
  constructor(scope: Construct, id: string, props?: StackProps) {
    super(scope, id, props);

    // The URL to the git repository to use for Flux
    const repoUrl = this.getContextOrDefault('FluxRepoURL', "https://github.com/awslabs/kubernetes-iteration-toolkit")
    const repoBranch = this.getContextOrDefault('FluxRepoBranch', 'main')
    const repoPath = this.getContextOrDefault('FluxRepoPath', './infrastructure/k8s-config/clusters/kit-infrastructure')
    const installEBSCSIDriverAddon = this.getContextOrDefault("AWSEBSCSIDriverAddon", "true")
    const installKarpenterAddon = this.getContextOrDefault('KarpenterAddon', "true")
    const installKitAddon = this.getContextOrDefault("KITAddon", "true")

    const testRepoName = this.node.tryGetContext('TestFluxRepoName')
    const testRepoUrl = this.node.tryGetContext('TestFluxRepoURL')
    const testRepoBranch = this.node.tryGetContext('TestFluxRepoBranch')
    const testRepoPath = this.node.tryGetContext('TestFluxRepoPath')
    const testSA = this.node.tryGetContext("TestServiceAccount")
    const testNS = this.node.tryGetContext("TestNamespace")

    // A VPC, including NAT GWs, IGWs, where we will run our cluster
    const vpc = new ec2.Vpc(this, 'VPC', {});

    // The IAM role that will be used by EKS
    const clusterRole = new iam.Role(this, 'ClusterRole', {
      assumedBy: new iam.ServicePrincipal('eks.amazonaws.com'),
      managedPolicies: [
        iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSClusterPolicy'),
        iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSVPCResourceController')
      ]
    });

    // The EKS cluster, without worker nodes as we'll add them later
    const cluster = new eks.Cluster(this, 'Cluster', {
      clusterName: id,
      vpc: vpc,
      role: clusterRole,
      version: eks.KubernetesVersion.V1_21,
      defaultCapacity: 0,
    });

    // Worker node IAM role
    const workerRole = new iam.Role(this, 'WorkerRole', {
      assumedBy: new iam.ServicePrincipal('ec2.amazonaws.com'),
      managedPolicies: [
        iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSWorkerNodePolicy'),
        iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEC2ContainerRegistryReadOnly'),
        iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKS_CNI_Policy'),
        iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSVPCResourceController'), // Allows us to use Security Groups for pods
        iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonSSMManagedInstanceCore')
      ]
    });

    cluster.awsAuth.addRoleMapping(workerRole, {
        username: 'system:node:{{EC2PrivateDNSName}}',
        groups: ['system:bootstrappers', 'system:nodes']
    })

    const sg = new SecurityGroup(this, "NodeSecurityGroup", {
      description: "Worker Node Security Group",
      vpc: vpc,
    });
    cluster.clusterSecurityGroup.addIngressRule(sg, ec2.Port.allTraffic())
    sg.addIngressRule(cluster.clusterSecurityGroup, ec2.Port.allTraffic())
    sg.addIngressRule(sg, ec2.Port.allTraffic())
    sg.addEgressRule(ec2.Peer.anyIpv4(), ec2.Port.allTraffic())
    sg.addEgressRule(ec2.Peer.anyIpv6(), ec2.Port.allTraffic())
    Tags.of(sg).add('kit.sh/stack', super.stackName)
    Tags.of(vpc).add('kit.sh/stack', super.stackName)

    // Select the private subnets created in our VPC and place our worker nodes there
    const privateSubnets = vpc.selectSubnets({
      subnetType: ec2.SubnetType.PRIVATE_WITH_NAT,
    });

    const ng = cluster.addNodegroupCapacity('SystemPool', {
      subnets: privateSubnets,
      nodeRole: workerRole,
      minSize: 3,
      maxSize: 3,
      instanceTypes: [
        new ec2.InstanceType('m5.large'),
        new ec2.InstanceType('m5a.large'),
        new ec2.InstanceType('m6i.large'),
        new ec2.InstanceType('m6a.large'),
        new ec2.InstanceType('t3.large'),
        new ec2.InstanceType('t3a.large'),
        new ec2.InstanceType('c5.large'),
        new ec2.InstanceType('c5a.large'),
        new ec2.InstanceType('c6i.large'),
      ],
      tags: {
        "kit.sh/stack": super.stackName,
      },
      taints: [
        {
          effect: eks.TaintEffect.NO_SCHEDULE,
          key: 'CriticalAddonsOnly',
          value: 'true',
        },
      ],
    });

    // Setup Tekton test permissions

    const ns = cluster.addManifest('tekton-tests-ns', {
      apiVersion: 'v1',
      kind: 'Namespace',
      metadata: {
          name: testNS
      }
    });

    const sa = cluster.addServiceAccount('tekton-tests-sa', {
        name: testSA,
        namespace: testNS
    })
    sa.node.addDependency(ns)
    sa.role.attachInlinePolicy(new iam.Policy(this, 'tekton-tests-policy', {
        statements: [
            new iam.PolicyStatement({
                resources: ['*'],
                actions: [
                    // Write Operations
                    "ec2:*",
                    "cloudformation:*",
                    "iam:*",
                    "ssm:GetParameter",
                    "eks:*",
                    "pricing:GetProducts",
                    "sts:AssumeRole",
                    "s3:*"
                ],
            }),
        ],
    }));

    cluster.awsAuth.addRoleMapping(sa.role, {
        username: 'system:node:{{EC2PrivateDNSName}}',
        groups: ['system:bootstrappers', 'system:nodes']
    })

    // Install cluster add-ons for the host cluster
    if (installEBSCSIDriverAddon == "true") {
      new AWSEBSCSIDriver(this, 'AWSEBSCSIDriver', {
        cluster: cluster,
        namespace: 'aws-ebs-csi-driver',
        version: 'v1.9.0',
        chartVersion: 'v2.8.1',
      }).node.addDependency(cluster);
    }

    new FluxV2(this, 'Flux', {
      cluster: cluster,
      namespace: 'flux-system',
      repoUrl: repoUrl,
      repoBranch: repoBranch,
      repoPath: repoPath,
      testRepoName: testRepoName,
      testRepoUrl: testRepoUrl,
      testRepoBranch: testRepoBranch,
      testRepoPath: testRepoPath,
      testNamespace: testNS,
    }).node.addDependency(cluster);

    new AWSLoadBalancerController(this, 'AWSLoadBalancerController', {
      cluster: cluster,
      namespace: 'aws-load-balancer-controller',
      version: 'v2.4.2',
    }).node.addDependency(cluster);

    if(installKitAddon == "true"){
      new KIT(this, 'KIT', {
        cluster: cluster,
        namespace: 'kit',
        version: 'v0.0.18',
      }).node.addDependency(cluster);
    }

    if(installKarpenterAddon == "true") {
      new Karpenter(this, 'KarpenterController', {
        cluster: cluster,
        namespace: 'karpenter',
        nodeRoleName: workerRole.roleName,
      }).node.addDependency(cluster);
    }
  }

  private getContextOrDefault(key: string, def: string | null): any {
    const val = this.node.tryGetContext(key)
    if (val === undefined) {
      return def
    } else {
      return val
    }
  }
}

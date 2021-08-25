import cdk = require('@aws-cdk/core')
import eks = require('@aws-cdk/aws-eks')
import iam = require('@aws-cdk/aws-iam')

export interface KarpenterProps {
    cluster: eks.Cluster
}

export class Karpenter extends cdk.Construct {
    constructor(scope: cdk.Construct, id: string, props: KarpenterProps) {
        super(scope, id)
        const namespace = "karpenter"
        const ns = props.cluster.addManifest('namespace', {
            apiVersion: 'v1',
            kind: 'Namespace',
            metadata: {
                name: namespace
            }
        })

        // Controller Role
        const sa = props.cluster.addServiceAccount('karpenter-controller-sa', {
            name: "karpenter",
            namespace: namespace
        })
        sa.node.addDependency(ns)
        sa.role.attachInlinePolicy(new iam.Policy(this, 'karpenter-controller-policy', {
            statements: [
                new iam.PolicyStatement({
                    resources: ['*'],
                    actions: ["ec2:CreateLaunchTemplate", "ec2:CreateFleet", "ec2:RunInstances",
                        "ec2:CreateTags", "iam:PassRole", "ec2:TerminateInstances", "ec2:DescribeLaunchTemplates",
                        "ec2:DescribeInstances", "ec2:DescribeSecurityGroups", "ec2:DescribeSubnets",
                        "ec2:DescribeInstanceTypes", "ec2:DescribeInstanceTypeOfferings", "ec2:DescribeAvailabilityZones",
                        "ssm:GetParameter"],
                }),
            ],
        }))

        // Node Role
        const nodeRole = new iam.Role(this, 'karpenter-node-role', {
            assumedBy: new iam.ServicePrincipal('ec2.amazonaws.com'),
            managedPolicies: [
                iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSWorkerNodePolicy'),
                iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEC2ContainerRegistryReadOnly'),
                iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKS_CNI_Policy'),
                iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonSSMManagedInstanceCore')
            ]
        })

        props.cluster.awsAuth.addRoleMapping(nodeRole, {
            username: 'system:node:{{EC2PrivateDNSName}}',
            groups: ['system:bootstrappers', 'system:nodes']
        })

        new iam.CfnInstanceProfile(this, 'karpenter-instance-profile', {
            roles: [nodeRole.roleName],
            instanceProfileName: `KarpenterNodeInstanceProfile-${props.cluster.clusterName}`
        })

        // Install Karpenter
        const chart = props.cluster.addHelmChart('karpenter', {
            chart: 'karpenter',
            release: 'karpenter',
            version: 'v0.3.1',
            repository: 'https://awslabs.github.io/karpenter/charts',
            namespace: namespace,
            createNamespace: false,
            values: {
                'serviceAccount': {
                    'create': false,
                    'name': sa.serviceAccountName,
                    'annotations': {
                        'eks.amazonaws.com/role-arn': sa.role.roleArn
                    }
                }
            }
        })
        chart.node.addDependency(ns)

        // Default Provisioner
        props.cluster.addManifest("default-provisioner", {
            apiVersion: 'karpenter.sh/v1alpha3',
            kind: 'Provisioner',
            metadata: {
                name: 'default',
            },
            spec: {
                cluster: {
                    name: props.cluster.clusterName,
                    endpoint: props.cluster.clusterEndpoint,
                },
                ttlSecondsAfterEmpty: 30,
            }
        }).node.addDependency(chart)
    }
}

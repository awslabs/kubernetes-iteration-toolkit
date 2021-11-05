import * as ec2 from '@aws-cdk/aws-ec2'
import * as eks from '@aws-cdk/aws-eks'
import * as iam from '@aws-cdk/aws-iam'
import * as cdk from '@aws-cdk/core'
import { Tags } from '@aws-cdk/core'
import { Addons } from './addons/construct'
import { RepositoryProps } from './addons/flux/construct'

export interface TestbedProps extends cdk.StackProps {
    repositories: RepositoryProps[]
}

export class Testbed extends cdk.Stack {
    constructor(scope: cdk.Construct, id: string = "testbed", props: TestbedProps) {
        super(scope, id, props)

        const vpc = new ec2.Vpc(this, id, {
            cidr: '10.0.0.0/16',
            maxAzs: 99,
            subnetConfiguration: [
                {
                    name: 'pub-subnet-1',
                    subnetType: ec2.SubnetType.PUBLIC,
                    cidrMask: 28,
                },
                {
                    name: 'priv-subnet-1',
                    subnetType: ec2.SubnetType.PRIVATE_WITH_NAT,
                    cidrMask: 28,
                },
            ],
        });

        //ToDo: revisit once this is resolved - https://github.com/aws/aws-cdk/issues/5927
        //create private subnets for KIT operator CP nodes/pods in all AZs
        for (let index = 0; index < cdk.Stack.of(this).availabilityZones.length; index++) {
            //Also, pick up non overlapping cidrs with KIT operator DP nodes;
            let privateSubnet = this.createPrivateSubnetForVPC(id, vpc, `10.${index + 20}.0.0/16`, cdk.Stack.of(this).availabilityZones[index])
            //Tag private subnets for KIT CP
            Tags.of(privateSubnet).add('kit/hostcluster', `${id}-controlplane`)
            let natSubnet = this.createPublicSubnetForVPC(id, vpc, `10.0.80.${index * 16}/28`, cdk.Stack.of(this).availabilityZones[index])
            this.configureNatProviderForPrivateSubnet(vpc, natSubnet, privateSubnet)
        }
        // index<=8 will give us 9  /16 cidrs additionally to make a mega VPC for DP nodes.
        for (let index = 0; index <= 8; index++) {
            let privateSubnet = this.createPrivateSubnetForVPC(id, vpc, `10.${index + 1}.0.0/16`, cdk.Stack.of(this).availabilityZones[index % cdk.Stack.of(this).availabilityZones.length])
            //Tag private subnets for KIT DP
            Tags.of(privateSubnet).add('kit/hostcluster', `${id}-dataplane`)
            let natSubnet = this.createPublicSubnetForVPC(id, vpc, `10.0.64.${index * 16}/28`, cdk.Stack.of(this).availabilityZones[index % cdk.Stack.of(this).availabilityZones.length])
            this.configureNatProviderForPrivateSubnet(vpc, natSubnet, privateSubnet)
        }

        const cluster = new eks.Cluster(this, 'cluster', {
            clusterName: id,
            vpc: vpc,
            role: new iam.Role(this, 'cluster-role', {
                assumedBy: new iam.ServicePrincipal('eks.amazonaws.com'),
                managedPolicies: [
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSClusterPolicy'),
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSVPCResourceController')
                ]
            }),
            version: eks.KubernetesVersion.V1_19,
            defaultCapacity: 0,
        })

        cluster.addNodegroupCapacity('node-group', {
            nodegroupName: 'default',
            subnets: vpc.selectSubnets({
                subnetType: ec2.SubnetType.PRIVATE_WITH_NAT
            }),
            nodeRole: new iam.Role(this, 'node-role', {
                assumedBy: new iam.ServicePrincipal('ec2.amazonaws.com'),
                managedPolicies: [
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSWorkerNodePolicy'),
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEC2ContainerRegistryReadOnly'),
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKS_CNI_Policy'),
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSVPCResourceController') // Allows us to use Security Groups for pods
                ]
            }),
        })
        // service account used by tekton workflows.
        cluster.addServiceAccount('test-executor', { name: 'test-executor' })
            .role.addManagedPolicy({ managedPolicyArn: 'arn:aws:iam::aws:policy/AdministratorAccess' })

        new Addons(this, `${id}-addons`, { cluster: cluster, repositories: props.repositories })

        // Tag all resources for discovery by Karpenter
        Tags.of(this).add(`kubernetes.io/cluster/${id}`, "owned")
    }

    createPrivateSubnetForVPC(id: string, vpc: ec2.Vpc, cidr: string, az: string): ec2.PrivateSubnet {
        let additionalCidr = new ec2.CfnVPCCidrBlock(this, `${id}-cidr-${cidr}`, {
            vpcId: vpc.vpcId,
            cidrBlock: cidr
        });
        let privateSubnet = new ec2.PrivateSubnet(this, `${id}-private-subnet-${cidr}`, {
            availabilityZone: az,
            vpcId: vpc.vpcId,
            cidrBlock: cidr
        })
        privateSubnet.node.addDependency(additionalCidr);
        return privateSubnet
    }
    createPublicSubnetForVPC(id: string, vpc: ec2.Vpc, cidr: string, az: string): ec2.PublicSubnet {
        let publicSubnet = new ec2.PublicSubnet(this, `${id}-nat-subnet-${cidr}`, {
            availabilityZone: az,
            vpcId: vpc.vpcId,
            cidrBlock: cidr
        })
        //add igw route for nat subnets
        let routeTableId = publicSubnet.routeTable.routeTableId
        new ec2.CfnRoute(this, `publicIGWRoute-${cidr}`, {
            routeTableId,
            gatewayId: vpc.internetGatewayId,
            destinationCidrBlock: "0.0.0.0/0"
        })
        return publicSubnet
    }
    configureNatProviderForPrivateSubnet(vpc: ec2.Vpc, natSubnet: ec2.PublicSubnet, privateSubnet: ec2.PrivateSubnet): void {
        ec2.NatProvider.gateway().configureNat({
            natSubnets: [
                natSubnet
            ],
            privateSubnets: [
                privateSubnet
            ],
            vpc: vpc
        })
    }
}

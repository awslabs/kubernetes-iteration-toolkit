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
    constructor(scope: cdk.Construct, id: string, props: TestbedProps) {
        super(scope, id)

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
        // index<=8 will give us 9  /16 cidrs additionally to make a mega VPC.
        for (let index = 0; index <= 8; index++) {
            let additionalCidr = new ec2.CfnVPCCidrBlock(this, `${id}-cidr-${index}`, {
                vpcId: vpc.vpcId,
                cidrBlock: `10.${index + 1}.0.0/16`
            });
            let privateSubnet = new ec2.PrivateSubnet(this, `${id}-private-subnet-${index}`, {
                availabilityZone: cdk.Stack.of(this).availabilityZones[index%cdk.Stack.of(this).availabilityZones.length],
                vpcId: vpc.vpcId,
                cidrBlock: `10.${index + 1}.0.0/16`
            })
            privateSubnet.node.addDependency(additionalCidr);

            ec2.NatProvider.gateway().configureNat({
                natSubnets: [
                    new ec2.PublicSubnet(this, `${id}-nat-subnet-${index}`, {
                        availabilityZone: cdk.Stack.of(this).availabilityZones[index%cdk.Stack.of(this).availabilityZones.length],
                        vpcId: vpc.vpcId,
                        cidrBlock: `10.0.64.${index*16}/28`
                    })
                ],
                privateSubnets: [
                    privateSubnet
                ],
                vpc: vpc
            })
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
}

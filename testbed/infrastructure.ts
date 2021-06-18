import * as iam from '@aws-cdk/aws-iam';
import * as eks from '@aws-cdk/aws-eks';
import * as ec2 from '@aws-cdk/aws-ec2';
import * as cdk from '@aws-cdk/core'
import { StackProps } from '@aws-cdk/core';

export class Infrastructure extends cdk.Construct {
    cluster: eks.Cluster;
    constructor(scope: cdk.Construct, id: string, props: StackProps) {
        super(scope, id);

        const vpc = new ec2.Vpc(this, 'VPC', {});

        this.cluster = new eks.Cluster(this, 'Cluster', {
            vpc: vpc,
            role: new iam.Role(this, 'ClusterRole', {
                assumedBy: new iam.ServicePrincipal('eks.amazonaws.com'),
                managedPolicies: [
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSClusterPolicy'),
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSVPCResourceController')
                ]
            }),
            version: eks.KubernetesVersion.V1_19,
            defaultCapacity: 0
        });

        // for time being we start with these defaults; To-do use karpenter to manage this
        this.cluster.addNodegroupCapacity('WorkerNodeGroup', {
            subnets: vpc.selectSubnets({
                subnetType: ec2.SubnetType.PRIVATE
            }),
            nodeRole: new iam.Role(this, 'WorkerRole', {
                assumedBy: new iam.ServicePrincipal('ec2.amazonaws.com'),
                managedPolicies: [
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSWorkerNodePolicy'),
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEC2ContainerRegistryReadOnly'),
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKS_CNI_Policy'),
                    iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonEKSVPCResourceController') // Allows us to use Security Groups for pods
                ]
            }),
            minSize: 5,
            maxSize: 20
        });
    }
}

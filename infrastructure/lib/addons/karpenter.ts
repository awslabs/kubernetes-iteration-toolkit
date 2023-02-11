import { aws_eks as eks, aws_iam as iam, Duration, StackProps } from 'aws-cdk-lib'
import { Construct } from 'constructs'

export interface KarpenterProps extends StackProps {
    cluster: eks.Cluster
    namespace: string
    nodeRoleName: string
}

export class Karpenter extends Construct {
    constructor(scope: Construct, id: string, props: KarpenterProps) {
        super(scope, id)
        const ns = props.cluster.addManifest('karpenter-namespace', {
            apiVersion: 'v1',
            kind: 'Namespace',
            metadata: {
                name: props.namespace
            }
        })

        // Controller Role
        const sa = props.cluster.addServiceAccount('karpenter-controller-sa', {
            name: "karpenter",
            namespace: props.namespace
        })
        sa.node.addDependency(ns)
        sa.role.attachInlinePolicy(new iam.Policy(this, 'karpenter-controller-policy', {
            statements: [
                new iam.PolicyStatement({
                    resources: ['*'],
                    actions: [
                        // Write Operations
                        "ec2:CreateLaunchTemplate",
                        "ec2:CreateFleet",
                        "ec2:RunInstances",
                        "ec2:CreateTags",
                        "iam:PassRole",
                        "ec2:TerminateInstances",
                        "ec2:DeleteLaunchTemplate",
                        // Read Operations
                        "ec2:DescribeAvailabilityZones",
                        "ec2:DescribeImages",
                        "ec2:DescribeInstances",
                        "ec2:DescribeInstanceTypeOfferings",
                        "ec2:DescribeInstanceTypes",
                        "ec2:DescribeLaunchTemplates",
                        "ec2:DescribeSecurityGroups",
                        "ec2:DescribeSpotPriceHistory",
                        "ec2:DescribeSubnets",
                        "pricing:GetProducts",
                        "ssm:GetParameter",
                    ],
                }),
            ],
        }));

        const nodeInstanceProfile = new iam.CfnInstanceProfile(this, 'karpenter-instance-profile', {
            roles: [props.nodeRoleName],
            instanceProfileName: `KarpenterNodeInstanceProfile-${props.cluster.clusterName}`
        });

        // Install Karpenter
        const chart = props.cluster.addHelmChart('karpenter-chart', {
            chart: 'karpenter',
            release: 'karpenter',
            version: 'v0.21.1',
            repository: 'oci://public.ecr.aws/karpenter/karpenter',
            namespace: props.namespace,
            createNamespace: false,
            timeout: Duration.minutes(10),
            wait: true,
            values: {
                'settings': {
                    'aws': {
                        'clusterName': props.cluster.clusterName,
                        'clusterEndpoint': props.cluster.clusterEndpoint,
                        'defaultInstanceProfile': nodeInstanceProfile.instanceProfileName,
                    },
                },
                'serviceAccount': {
                    'create': false,
                    'name': sa.serviceAccountName,
                },
            },
        })
        chart.node.addDependency(sa)
    }
}

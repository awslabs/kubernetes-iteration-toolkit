import { Construct } from 'constructs';
import { aws_iam as iam,Duration, CfnResource, StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';

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
                        "ec2:DescribeLaunchTemplates",
                        "ec2:DescribeInstances",
                        "ec2:DescribeSecurityGroups",
                        "ec2:DescribeSubnets",
                        "ec2:DescribeInstanceTypes",
                        "ec2:DescribeInstanceTypeOfferings",
                        "ec2:DescribeAvailabilityZones",
                        "ec2:DescribeSpotPriceHistory",
                        "ec2:DescribeImages",
                        "ssm:GetParameter",
                        "pricing:GetProducts",
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
            version: 'v0.16.1',
            repository: 'https://charts.karpenter.sh',
            namespace: props.namespace,
            createNamespace: false,
            timeout: Duration.minutes(10),
            wait: true,
            values: {
                'clusterName': props.cluster.clusterName,
                'clusterEndpoint': props.cluster.clusterEndpoint,
                'aws': {
                    'defaultInstanceProfile': nodeInstanceProfile.instanceProfileName,
                },
                'serviceAccount': {
                    'create': false,
                    'name': sa.serviceAccountName,
                },
                tolerations: [
                    {
                        key: 'CriticalAddonsOnly',
                        operator: 'Exists',
                    },
                ],
            }
        })
        chart.node.addDependency(sa)
    }
}

import { Construct } from 'constructs';
import { aws_iam as iam, Stack, StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';

export interface AWSFluentBitProps extends StackProps {
    cluster: eks.Cluster
    namespace: string
    version: string
    chartVersion: string
}

export class AWSFluentBit extends Construct {
    constructor(scope: Construct, id: string, props: AWSFluentBitProps) {
        super(scope, id)
        const ns = props.cluster.addManifest('aws-fluent-bit-namespace', {
            apiVersion: 'v1',
            kind: 'Namespace',
            metadata: {
                name: props.namespace
            }
        })

        // Controller Role
        const sa = props.cluster.addServiceAccount('aws-fluent-bit-sa', {
            name: "aws-fluent-bit",
            namespace: props.namespace
        })
        sa.node.addDependency(ns)
        sa.role.addManagedPolicy(iam.ManagedPolicy.fromAwsManagedPolicyName('CloudWatchAgentServerPolicy'))

        const chart = props.cluster.addHelmChart('aws-fluent-bit-chart', {
            chart: 'aws-for-fluent-bit',
            release: 'aws-fluent-bit',
            version: props.chartVersion,
            repository: 'https://aws.github.io/eks-charts',
            namespace: props.namespace,
            createNamespace: false,
            values: {
                serviceAccount: {
                    create: "false",
                    name: sa.serviceAccountName,
                },
                cloudWatch: {
                  region: Stack.of(this).region,
                  logRetentionDays: "90",
                },
                firehose: {
                  enabled: "false",
                },
                kinesis: {
                  enabled: "false",
                },
                elasticsearch: {
                  enabled: "false",
                },
                tolerations: [
                    {
                        key: 'CriticalAddonsOnly',
                        operator: 'Exists',
                    },
                ],
            }
        })
        chart.node.addDependency(ns)
    }
}

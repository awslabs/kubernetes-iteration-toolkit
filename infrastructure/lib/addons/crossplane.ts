import { Construct } from 'constructs';
import { aws_iam as iam, Duration, StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';

export interface CrossplaneProps extends StackProps {
    cluster: eks.Cluster
    namespace: string
    version: string
}

export class Crossplane extends Construct {
    constructor(scope: Construct, id: string, props: CrossplaneProps) {
        super(scope, id)
        const ns = props.cluster.addManifest('crossplane-namespace', {
            apiVersion: 'v1',
            kind: 'Namespace',
            metadata: {
                name: props.namespace
            }
        })

        // Controller Role
        const sa = props.cluster.addServiceAccount('crossplane-controller-sa', {
            name: "crossplane-aws-irsa",
            namespace: props.namespace
        })
        sa.node.addDependency(ns)
        sa.role.attachInlinePolicy(new iam.Policy(this, 'crossplane-aws-policy', {
            statements: [
                new iam.PolicyStatement({
                    resources: ['*'],
                    actions: [
                        // Write Operations
                        "iam:*",
                        "sts:*",
                    ],
                }),
            ],
        }))

        const chart = props.cluster.addHelmChart('crossplane-chart', {
            chart: 'crossplane',
            release: 'crossplane',
            version: props.version,
            repository: 'https://charts.crossplane.io/stable',
            namespace: props.namespace,
            createNamespace: false,
            timeout: Duration.minutes(10),
            wait: true,
            values: {
                tolerations: [
                    {
                        key: 'CriticalAddonsOnly',
                        operator: 'Exists',
                    },
                ],
                rbacManager: {
                    tolerations: [
                        {
                            key: 'CriticalAddonsOnly',
                            operator: 'Exists',
                        },
                    ],
                }
            }
        })
        chart.node.addDependency(ns)

        const controllerConfig = props.cluster.addManifest("crossplane-controller-config", {
            apiVersion: 'pkg.crossplane.io/v1alpha1',
            kind: 'ControllerConfig',
            metadata: {
                name: 'aws-config',
                annotations: {
                    'eks.amazonaws.com/role-arn': sa.role.roleArn
                }
            },
            spec: {
                podSecurityContext: {
                    'fsGroup': 2000
                },
                tolerations: [
                    {
                        key: 'CriticalAddonsOnly',
                        operator: 'Exists',
                    },
                ],
            },
        });
        controllerConfig.node.addDependency(chart)


        const providerManifest = props.cluster.addManifest("crossplane-aws-provider", {
            apiVersion: 'pkg.crossplane.io/v1',
            kind: 'Provider',
            metadata: {
                name: 'provider-aws',
            },
            spec: {
                package: 'crossplane/provider-aws:v0.15.0',
                controllerConfigRef: {
                    name: 'aws-config',
                },
            },
        });
        providerManifest.node.addDependency(chart)

        // TODO: need to wait for the provider to come up, but can probably do this in flux
        // const awsProviderCRDs = new eks.KubernetesObjectValue(this, 'crossplane-aws-provider-crds', {
        //     cluster: props.cluster,
        //     objectType: 'crd',
        //     objectName: 'providerconfigs.aws.crossplane.io',
        //     jsonPath: '.',
        //     timeout: Duration.minutes(5),
        // })
        // awsProviderCRDs.node.addDependency(providerManifest)

        // const providerConfig = props.cluster.addManifest("crossplane-provider-config", {
        //     apiVersion: 'aws.crossplane.io/v1beta1',
        //     kind: 'ProviderConfig',
        //     metadata: {
        //         name: 'crossplane-provider-config',
        //     },
        //     spec: {
        //         credentials: {
        //             source: 'InjectedIdentity',
        //         },
        //     },
        // });
        // providerConfig.node.addDependency(awsProviderCRDs)
    }
}

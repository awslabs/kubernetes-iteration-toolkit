import cdk = require('@aws-cdk/core')
import eks = require('@aws-cdk/aws-eks')
import iam = require('@aws-cdk/aws-iam')

export interface KitProps {
    cluster: eks.Cluster
}

export class Kit extends cdk.Construct {
    constructor(scope: cdk.Construct, id: string, props: KitProps) {
        super(scope, id)
        const namespace = "kit"
        const ns = props.cluster.addManifest('kit-namespace', {
            apiVersion: 'v1',
            kind: 'Namespace',
            metadata: {
                name: namespace
            }
        })

        // Controller Role
        const sa = props.cluster.addServiceAccount('kit-controller-sa', {
            name: "kit-controller",
            namespace: namespace
        })
        sa.node.addDependency(ns)
        sa.role.attachInlinePolicy(new iam.Policy(this, 'kit-controller-policy', {
            statements: [
                new iam.PolicyStatement({
                    resources: ['*'],
                    actions: [
                        // Write Operations
                        "ec2:CreateTags",
                        "ec2:CreateLaunchTemplate",
                        "ec2:CreateLaunchTemplateVersion",
                        "ec2:DeleteLaunchTemplate",
                        "ec2:RunInstances",
                        "iam:passRole",
                        "autoscaling:CreateOrUpdateTags",
                        "autoscaling:CreateAutoScalingGroup",
                        "autoscaling:DeleteAutoScalingGroup",
                        "autoscaling:UpdateAutoScalingGroup",
                        "autoscaling:SetDesiredCapacity",
                        //Read Operations
                        "ec2:DescribeInstances",
                        "ec2:DescribeLaunchTemplates",
                        "ec2:DescribeLaunchTemplateVersions",
                        "ec2:DescribeSubnets",
                        "ssm:GetParameter",
                        "autoscaling:DescribeAutoScalingGroups"]
                }),
            ],
        }))

        // Install kit
        const chart = props.cluster.addHelmChart('kit', {
            chart: 'kit-operator',
            release: 'kit-operator',
            repository: 'https://awslabs.github.io/kubernetes-iteration-toolkit/',
            namespace: namespace,
            createNamespace: false,
            values: {
                'serviceAccount': {
                    'create': false,
                    'name': sa.serviceAccountName,
                    'annotations': {
                        'eks.amazonaws.com/role-arn': sa.role.roleArn
                    }
                },

            }
        })
        chart.node.addDependency(ns)

        //Karp Provisioner for kit
        props.cluster.addManifest("default-provisioner", {
            apiVersion: 'karpenter.sh/v1alpha5',
            kind: 'Provisioner',
            metadata: {
                name: 'default',
            },
            spec: {
                provider: {
                    cluster: {
                        name: props.cluster.clusterName,
                        endpoint: props.cluster.clusterEndpoint,
                    },
                    subnetSelector: {
                        "kit/hostcluster": `${props.cluster.clusterName}-controlplane`
                    }
                },
                ttlSecondsAfterEmpty: 30,
            }
        })

    }
}

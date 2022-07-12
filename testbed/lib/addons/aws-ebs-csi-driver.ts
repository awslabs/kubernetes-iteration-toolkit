import { Construct } from 'constructs';
import { aws_iam as iam, StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';
import * as request from 'sync-request';

export interface AWSEBSCSIDriverProps extends StackProps {
    cluster: eks.Cluster
    namespace: string
    version: string
    chartVersion: string
}

export class AWSEBSCSIDriver extends Construct {
    constructor(scope: Construct, id: string, props: AWSEBSCSIDriverProps) {
        super(scope, id)
        const ns = props.cluster.addManifest('aws-ebs-csi-namespace', {
            apiVersion: 'v1',
            kind: 'Namespace',
            metadata: {
                name: props.namespace
            }
        })

        // Controller Role
        const sa = props.cluster.addServiceAccount('aws-ebs-csi-driver-sa', {
            name: "aws-ebs-csi-driver",
            namespace: props.namespace
        })
        sa.node.addDependency(ns)
        sa.role.attachInlinePolicy(new iam.Policy(this, 'aws-ebs-csi-driver-policy', {document: iam.PolicyDocument.fromJson(this.getIAMPolicy(props.version))}))

        // Install Karpenter
        const chart = props.cluster.addHelmChart('aws-ebs-csi-driver-chart', {
            chart: 'aws-ebs-csi-driver',
            release: 'aws-ebs-csi-driver',
            version: props.chartVersion,
            repository: 'https://kubernetes-sigs.github.io/aws-ebs-csi-driver',
            namespace: props.namespace,
            createNamespace: false,
            values: {
                'controller': {
                    'replicaCount': 1,
                    'serviceAccount': {
                        'create': false,
                        'name': sa.serviceAccountName,
                        'annotations': {
                            'eks.amazonaws.com/role-arn': sa.role.roleArn
                        },
                    },
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
    private getIAMPolicy(version: string): any {
        const metadataUrl = `https://raw.githubusercontent.com/kubernetes-sigs/aws-ebs-csi-driver/${version}/docs/example-iam-policy.json`;
        return JSON.parse(
          request.default('GET', metadataUrl, {
            headers: {
              'User-Agent': 'CDK' // GH API requires us to set UA
            }
          }).getBody().toString()
        );
      }
}

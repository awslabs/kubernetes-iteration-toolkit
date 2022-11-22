import { Construct } from 'constructs';
import { aws_iam as iam, StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';
import * as request from 'sync-request';
import * as fs from 'fs';

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
        // Update and run REPO_DIR/cache-iam-policies.sh to download and cache this policy
        return JSON.parse(
            fs.readFileSync(`lib/addons/cached/aws-ebs-csi-driver-iam-policy-${version}.json`,'utf8')
        );
      }
}

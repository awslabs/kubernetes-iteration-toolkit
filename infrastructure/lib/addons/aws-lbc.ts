import { Construct } from 'constructs';
import { Aws, StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';
import { aws_iam as iam } from 'aws-cdk-lib';
import * as request from 'sync-request';
import * as fs from 'fs';

export interface AWSLoadBalancerControllerProps extends StackProps {
  cluster: eks.Cluster;
  namespace: string;
  version: string;
}

export class AWSLoadBalancerController extends Construct {
  constructor(scope: Construct, id: string, props: AWSLoadBalancerControllerProps) {
    super(scope, id);

    const ns = props.cluster.addManifest('aws-lbc-namespace', {
      apiVersion: 'v1',
      kind: 'Namespace',
      metadata: {
          name: props.namespace
      }
  })

    const sa = props.cluster.addServiceAccount('aws-lbc-sa', {
      name: 'aws-load-balancer-controller',
      namespace: props.namespace
    });
    sa.node.addDependency(ns)
    sa.role.attachInlinePolicy(new iam.Policy(this, 'aws-lbc-policy', {document: iam.PolicyDocument.fromJson(this.getIAMPolicy(props.version))}))

    const chart = props.cluster.addHelmChart('AWSLBCHelmChart', {
      chart: 'aws-load-balancer-controller',
      release: 'aws-load-balancer-controller',
      repository: 'https://aws.github.io/eks-charts',
      namespace: props.namespace,
      createNamespace: false,
      values: {
        'clusterName': `${props.cluster.clusterName}`,
        'serviceAccount': {
          'create': false,
          'name': sa.serviceAccountName,
          'annotations': {
            'eks.amazonaws.com/role-arn': sa.role.roleArn
          }
        },
        clusterSecretsPermissions: {
          allowAllSecrets: true  
        },
        tolerations: [
          {
              key: 'CriticalAddonsOnly',
              operator: 'Exists',
          },
        ],
      }
    });
    chart.node.addDependency(ns)
  }
  private getIAMPolicy(version: string): any {
      // Update and run REPO_DIR/cache-iam-policies.sh to download and cache this policy
      return JSON.parse(
          fs.readFileSync(`lib/addons/cached/aws-load-balancer-controller-iam-policy-${version}.json`,'utf8')
      );
  }
}

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
  useCachedIAMPolicy: boolean;
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
    sa.role.attachInlinePolicy(new iam.Policy(this, 'aws-lbc-policy', {document: iam.PolicyDocument.fromJson(this.getIAMPolicy(props.version, props.useCachedIAMPolicy))}))

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
  private getIAMPolicy(version: string, useCachedIAMPolicy: boolean): any {
    if (useCachedIAMPolicy){
      return JSON.parse(
          fs.readFileSync(`./cached/aws-load-balancer-controller-iam-policy-${version}.json`,'utf8')
      );
    }
    const metadataUrl = `https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/${version}/docs/install/iam_policy.json`;
    return JSON.parse(
      request.default('GET', metadataUrl, {
        headers: {
          'User-Agent': 'CDK' // GH API requires us to set UA
        }
      }).getBody().toString()
    );
  }
}
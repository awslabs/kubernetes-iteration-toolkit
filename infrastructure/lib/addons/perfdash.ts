import { Construct } from 'constructs';
import { StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';
import { aws_iam as iam } from 'aws-cdk-lib';

export interface PerfDashProps extends StackProps {
  cluster: eks.Cluster;
  namespace: string;
}

export class PerfDash extends Construct {
  constructor(scope: Construct, id: string, props: PerfDashProps) {
    super(scope, id);

    const ns = props.cluster.addManifest('perfdash-namespace', {
      apiVersion: 'v1',
      kind: 'Namespace',
      metadata: {
          name: props.namespace
      }
    })

    const sa = props.cluster.addServiceAccount('perfdash-sa', {
      name: 'perfdash-log-fetcher',
      namespace: props.namespace
    });
    sa.node.addDependency(ns)
    sa.role.attachInlinePolicy(new iam.Policy(this, 'perfdash-policy', {
      statements: [
        new iam.PolicyStatement({
            resources: ['*'],
            actions: [
              // S3 readonly access
              "s3:Get*",
              "s3:List*",
              "s3-object-lambda:Get*",
              "s3-object-lambda:List*",
            ],
        }),
      ],
    }));

    const perfdashKustomizationManifest = props.cluster.addManifest('PerfdashKustomizationSelf', {
      apiVersion: 'kustomize.toolkit.fluxcd.io/v1beta1',
      kind: 'Kustomization',
      metadata: {
        name: 'flux-addon-perfdash',
        namespace: props.namespace,
      },
      spec: {
        interval: '5m0s',
        path: "./infrastructure/k8s-config/clusters/kit-infrastructure/perfdash",
        prune: true,
        sourceRef: {
          kind: 'GitRepository',
          name: 'flux-system',
          namespace: 'flux-system'
        },
        validation: 'client',
        patches: [
          {
            target: {
              kind: "Deployment",
              name: "perfdash",
              namespace: "perfdash",
            },
            patch: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: perfdash
  namespace: perfdash
spec:
  template:
    spec:
      containers:
      - name: perfdash
        env:
        - name: AWS_ROLE_ARN
          value: `+sa.role.roleArn
          },
        ]
      }
    });
    perfdashKustomizationManifest.node.addDependency(ns);
    perfdashKustomizationManifest.node.addDependency(sa);
  }
}
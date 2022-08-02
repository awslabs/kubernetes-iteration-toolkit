import { Construct } from 'constructs';
import { aws_iam as iam, Duration, StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';
import { PropagatedTagSource } from 'aws-cdk-lib/aws-ecs';

export interface FluxV2Props extends StackProps {
  cluster: eks.Cluster;
  namespace: string;
  fluxVersion?: string;
  repoUrl: string;
  repoBranch: string;
  repoPath: string;
  testRepoName: string;
  testRepoUrl?: string;
  testRepoBranch?: string;
  testRepoPath?: string;
  testNamespace?: string;
}
export class FluxV2 extends Construct {
  constructor(scope: Construct, id: string, props: FluxV2Props) {
    super(scope, id);
    
    const chart = props.cluster.addHelmChart('flux-chart', {
      chart: 'flux2',
      release: 'flux2',
      version: '1.0.0',
      repository: 'https://fluxcd-community.github.io/helm-charts',
      namespace: props.namespace,
      createNamespace: true,
      timeout: Duration.minutes(10),
      wait: true,
      values: {
          cli: {
            tolerations: [
                {
                    key: 'CriticalAddonsOnly',
                    operator: 'Exists',
                },
            ],
          },
          helmcontroller: {
            tolerations: [
                {
                    key: 'CriticalAddonsOnly',
                    operator: 'Exists',
                },
            ],
          },
          imageautomationcontroller: {
            tolerations: [
                {
                    key: 'CriticalAddonsOnly',
                    operator: 'Exists',
                },
            ],
          },
          imagereflectorcontroller: {
            tolerations: [
                {
                    key: 'CriticalAddonsOnly',
                    operator: 'Exists',
                },
            ],
          },
          kustomizecontroller: {
            tolerations: [
                {
                    key: 'CriticalAddonsOnly',
                    operator: 'Exists',
                },
            ],
          },
          notificationcontroller: {
            tolerations: [
                {
                    key: 'CriticalAddonsOnly',
                    operator: 'Exists',
                },
            ],
          },
          sourcecontroller: {
            tolerations: [
                {
                    key: 'CriticalAddonsOnly',
                    operator: 'Exists',
                },
            ],
          },
      }
  });

  // Bootstrap manifests
  const gitRepoManifest = props.cluster.addManifest('GitRepoSelf', {
    apiVersion: 'source.toolkit.fluxcd.io/v1beta1',
    kind: 'GitRepository',
    metadata: {
      name: 'flux-system',
      namespace: props.namespace
    },
    spec: {
      interval: '2m0s',
      ref: {
        branch: props.repoBranch,
      },
      url: props.repoUrl
    }  
  });
  gitRepoManifest.node.addDependency(chart);
  
  const kustomizationManifest = props.cluster.addManifest('KustomizationSelf', {
    apiVersion: 'kustomize.toolkit.fluxcd.io/v1beta1',
    kind: 'Kustomization',
    metadata: {
      name: 'flux-system',
      namespace: props.namespace,
    },
    spec: {
      interval: '2m0s',
      path: props.repoPath,
      prune: true,
      sourceRef: {
        kind: 'GitRepository',
        name: 'flux-system'
      },
      validation: 'client'
    }
  });
  kustomizationManifest.node.addDependency(chart);

  if (props.testRepoUrl) {
    const testGitRepoManifest = props.cluster.addManifest('TestGitRepoSelf', {
      apiVersion: 'source.toolkit.fluxcd.io/v1beta1',
      kind: 'GitRepository',
      metadata: {
        name: props.testRepoName,
        namespace: props.testNamespace
      },
      spec: {
        interval: '2m0s',
        ref: {
          branch: props.testRepoBranch,
        },
        url: props.testRepoUrl
      }  
    });
    testGitRepoManifest.node.addDependency(chart);
  }
  
  if (props.testRepoPath) {
    const testKustomizationManifest = props.cluster.addManifest('TestKustomizationSelf', {
      apiVersion: 'kustomize.toolkit.fluxcd.io/v1beta1',
      kind: 'Kustomization',
      metadata: {
        name: props.testRepoName,
        namespace: props.testNamespace,
      },
      spec: {
        interval: '2m0s',
        path: props.testRepoPath,
        prune: true,
        sourceRef: {
          kind: 'GitRepository',
          name: props.testRepoName,
        },
        validation: 'client'
      }
    });
    testKustomizationManifest.node.addDependency(chart);
  }

}}

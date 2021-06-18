import cdk = require('@aws-cdk/core');
import eks = require('@aws-cdk/aws-eks');
import * as yaml from 'js-yaml';
import * as request from 'sync-request';

export interface FluxProps {
    cluster: eks.Cluster;
    repositories: RepositoryProps[];
}

export interface RepositoryProps {
    url: string;
    name: string;
    branch?: string;
    path?: string;
}

export class Flux extends cdk.Construct {
    constructor(scope: cdk.Construct, id: string, props: FluxProps) {
        super(scope, id);

        const fluxManifest = props.cluster.addManifest(
            'flux', ...yaml.loadAll(request.default(
                "GET", "https://github.com/fluxcd/flux2/releases/download/v0.15.0/install.yaml").getBody().toString()));

        props.repositories.forEach(function (value) {
            // Bootstrap manifests
            const gitRepoManifest = props.cluster.addManifest('GitRepoSelf', {
                apiVersion: 'source.toolkit.fluxcd.io/v1beta1',
                kind: 'GitRepository',
                metadata: {
                    name: value.name,
                    namespace: 'default'
                },
                spec: {
                    // we can adjust this later if we want to be more aggressive  
                    interval: '5m0s',
                    ref: {
                        branch: value.branch ?? "main",
                    },
                    secretRef: {
                        name: 'github-key'
                    },
                    url: value.url
                }
            });
            gitRepoManifest.node.addDependency(fluxManifest);
            const kustomizationManifest = props.cluster.addManifest('KustomizationSelf', {
                apiVersion: 'kustomize.toolkit.fluxcd.io/v1beta1',
                kind: 'Kustomization',
                metadata: {
                    name: value.name,
                    namespace: 'default'
                },
                spec: {
                    // we can adjust this later if we want to be more aggressive  
                    interval: '5m0s',
                    path: value.path ?? "test/workflows",
                    prune: true,
                    sourceRef: {
                        kind: 'GitRepository',
                        name: value.name
                    },
                    validation: 'client'
                }
            });
            kustomizationManifest.node.addDependency(fluxManifest);
        });
    }
}

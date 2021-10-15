import cdk = require('@aws-cdk/core');
import eks = require('@aws-cdk/aws-eks');
import iam = require('@aws-cdk/aws-iam');
import * as yaml from 'js-yaml';
import * as request from 'sync-request';

export interface AWSLoadBalancerControllerProps {
    cluster: eks.Cluster;
    namespace: string;
}

export class AWSLoadBalancerController extends cdk.Construct {
    constructor(scope: cdk.Construct, id: string, props: AWSLoadBalancerControllerProps) {
        super(scope, id);

        const sa = props.cluster.addServiceAccount('aws-load-balancer-controller', {
            name: "aws-load-balancer-controller",
            namespace: props.namespace
        });

        sa.role.addManagedPolicy({
            //TODO: Use a managed policy - https://github.com/awslabs/kubernetes-iteration-toolkit/issues/6
            managedPolicyArn: `arn:aws:iam::aws:policy/AdministratorAccess`
        });

        const manifest = props.cluster.addManifest('awsLbcCrdManifest', ...yaml.loadAll(request.default('GET', 'https://raw.githubusercontent.com/aws/eks-charts/master/stable/aws-load-balancer-controller/crds/crds.yaml').getBody().toString()) as [Record<string,unknown>]);

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
                }
            }
        });
        chart.node.addDependency(manifest);
    }
}

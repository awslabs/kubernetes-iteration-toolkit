import { Construct } from 'constructs';
import { Aws, CfnStack, StackProps } from 'aws-cdk-lib';
import { aws_eks as eks } from 'aws-cdk-lib';
import { aws_iam as iam } from 'aws-cdk-lib';
import * as request from 'sync-request';
import { CfnInclude } from 'aws-cdk-lib/cloudformation-include';

export interface KITProps extends StackProps {
  cluster: eks.Cluster;
  namespace: string;
  version: string;
}

export class KIT extends Construct {
  constructor(scope: Construct, id: string, props: KITProps) {
    super(scope, id);

    const ns = props.cluster.addManifest('kit-namespace', {
      apiVersion: 'v1',
      kind: 'Namespace',
      metadata: {
          name: props.namespace
      }
  })

    const sa = props.cluster.addServiceAccount('kit-sa', {
      name: 'kit',
      namespace: props.namespace
    });

    // const kitPermissionsStack = new CfnInclude(this, 'kit-permissions-cfn', {
    //   templateFile: this.getCFN(props.version),
    // })
    sa.node.addDependency(ns)
    //sa.role.addManagedPolicy(iam.ManagedPolicy.fromManagedPolicyName(this, 'kit-managed-policy', `KitControllerPolicy-${props.cluster.clusterName}`))
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
                "iam:CreateRole",
                "iam:AddRoleToInstanceProfile",
                "iam:CreateInstanceProfile",
                "iam:AttachRolePolicy",
                "iam:RemoveRoleFromInstanceProfile",
                "iam:DeleteInstanceProfile",
                "iam:DetachRolePolicy",
                "iam:DeleteRole",
                "iam:TagRole",
                // Read Operations
                "ec2:DescribeInstances",
                "ec2:DescribeLaunchTemplates",
                "ec2:DescribeLaunchTemplateVersions",
                "ec2:DescribeSubnets",
                "ssm:GetParameter",
                "autoscaling:DescribeAutoScalingGroups",
                "iam:GetRole",
                "iam:GetInstanceProfile",
              ],
          }),
      ],
  }));

    const chart = props.cluster.addHelmChart('kit-chart', {
      chart: 'kit-operator',
      release: 'kit',
      repository: 'https://awslabs.github.io/kubernetes-iteration-toolkit',
      namespace: props.namespace,
      createNamespace: false,
      values: {
        'serviceAccount': {
          'create': false,
          'name': sa.serviceAccountName,
          'annotations': {
            'eks.amazonaws.com/role-arn': sa.role.roleArn
          }
        },
        controller: {
          tolerations: [
            {
                key: 'CriticalAddonsOnly',
                operator: 'Exists',
            },
          ],
        },
        webhook: {
          tolerations: [
            {
                key: 'CriticalAddonsOnly',
                operator: 'Exists',
            },
          ],
        },
      }
    });
    chart.node.addDependency(sa)
  }
}
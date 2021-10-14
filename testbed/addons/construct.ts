import cdk = require('@aws-cdk/core')
import * as eks from '@aws-cdk/aws-eks'
import { AWSLoadBalancerController } from './awslb/construct'
import { Flux, RepositoryProps } from './flux/construct'
import { Karpenter } from './karpenter/construct'
import { Kit } from './kit/construct'

export interface AddonsProps {
  cluster: eks.Cluster
  repositories: RepositoryProps[]
}

export class Addons extends cdk.Construct {
  constructor(scope: cdk.Construct, id: string, props: AddonsProps) {
    super(scope, id)

    new Flux(this, 'flux', {
      cluster: props.cluster,
      repositories: props.repositories,
    })

    new AWSLoadBalancerController(this, 'aws-load-balancer-controller', {
      cluster: props.cluster,
      namespace: 'kube-system'
    })

    new Karpenter(this, 'karpenter', {
      cluster: props.cluster
    })

    new Kit(this, 'kit', {
      cluster: props.cluster
    })
  }
}

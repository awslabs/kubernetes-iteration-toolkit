import cdk = require('@aws-cdk/core');
import * as eks from '@aws-cdk/aws-eks';
import { Flux, RepositoryProps } from './addons/flux/construct';

export interface AddonsProps {
  cluster: eks.Cluster
  repositories: RepositoryProps[];
}

export class Addons extends cdk.Construct {
  constructor(scope: cdk.Construct, id: string, props: AddonsProps) {
    super(scope, id);

    new Flux(this, 'Flux', {
      cluster: props.cluster,
      repositories: props.repositories,
    });
  }
}

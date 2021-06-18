import { Cluster } from '@aws-cdk/aws-eks';
import * as cdk from '@aws-cdk/core'
import { StackProps } from '@aws-cdk/core';
import { Addons } from './addons';
import { RepositoryProps } from './addons/flux/construct';
import { Infrastructure } from './infrastructure';

export interface TestbedProps extends cdk.StackProps {
    repositories: RepositoryProps[];
}

export class Testbed extends cdk.Construct {
    constructor(scope: cdk.Construct, id: string, props: TestbedProps) {
        super(scope, id);
        new Addons(this, 'addons', { cluster: new Infrastructure(this, 'eks', props).cluster, repositories: props.repositories })
    }
}

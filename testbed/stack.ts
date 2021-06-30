import * as cdk from '@aws-cdk/core'
import { Testbed, TestbedProps } from './construct';

export class TestbedStack extends cdk.Stack {
  constructor(scope: cdk.Construct, id: string, props: TestbedProps) {
    super(scope, id, props);
    new Testbed(this, "Testbed", props)
  }
}

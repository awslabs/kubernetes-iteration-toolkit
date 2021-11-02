#!/usr/bin/env node
import * as cdk from '@aws-cdk/core'
import { Testbed } from '../stack'

new Testbed(new cdk.App(),
    process.env.STACK_NAME, {
    env: {
        account: process.env.CDK_DEFAULT_ACCOUNT,
        region: process.env.CDK_DEFAULT_REGION
    },
    stackName: process.env.STACK_NAME,
    repositories: [
        { name: "testbed", url: "https://github.com/awslabs/kubernetes-iteration-toolkit", path: "./testbed/addons" },
        { name: "tests", url: "https://github.com/awslabs/kubernetes-iteration-toolkit", path: "./tests" },
        //To-do: move to a different repository
        //{ name: "triggers", url: "https://github.com/awslabs/kubernetes-iteration-toolkit", path: "./tests/triggers" }

        ]
});

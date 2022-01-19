# Testbed
 
## Setup

- Before installing the testbed in your AWS account increase the number of CIDR quota limit in a VPC from 5(default) to 12

The following create a Testbed cluster.
```
 npm install -g aws-cdk@latest
 npm install
 cdk bootstrap --profile <AWS-account-profile>
 STACK_NAME=my-testbed cdk deploy --profile <AWS-account-profile>
```

Once the `cdk deploy` command completes it prints the command `aws eks update-kubeconfig ...` to fetch the kube config for the testbed cluster

## Caveats
By default it creates two security groups which are tagged as `kubernetes.io/cluster/<testbed-name>:owned`
We need to delete the security group with no inbound rules

## Git config
TODO

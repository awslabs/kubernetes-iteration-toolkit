# KITCLI

## Installing KITCLI

```bash
brew tap awslabs/kit https://github.com/awslabs/kubernetes-iteration-toolkit.git
brew install kitcli
```

## Usage

```sh
kitcli apply
kitcli delete
```

```shell
cat <<EOF | kitcli apply -f -
apiVersion: kit.sh/v1alpha1
kind: Substrate
metadata:
  name: ${USER}
spec:
  instanceType: c5.2xlarge
---
apiVersion: kit.sh/v1alpha1
kind: Cluster
metadata:
  name: ${USER}
spec: {}
EOF

kitcli delete substrate ${USER}
```

## Developing
```
alias kitcli="go run ./cmd"
```

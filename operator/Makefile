RELEASE_REPO ?= public.ecr.aws/i7d3g4r4/kit-operator
RELEASE_VERSION ?= $(shell git describe --tags --always)

## Inject the app version into project.Version
LDFLAGS ?= "-ldflags=-X=github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/project.Version=$(RELEASE_VERSION)"
GOFLAGS ?= "$(LDFLAGS)"
WITH_GOFLAGS = GOFLAGS=$(GOFLAGS)
WITH_RELEASE_REPO = KO_DOCKER_REPO=$(RELEASE_REPO)
KIT_NAMESPACE ?= kit
CONTAINER_IMAGE_REGISTRY ?= 
## Extra helm options
HELM_OPTS ?= --set serviceAccount.create=false

help: ## Display help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

dev: verify test ## Run all steps in the developer loop

ci: verify licenses battletest ## Run all steps used by continuous integration

release: publish helm docs ## Run all steps in release workflow

test: ## Run tests
	ginkgo -r

build:
	go build $(GOFLAGS) -o bin/operator cmd/controller/main.go

battletest: ## Run stronger tests
	# Ensure all files have cyclo-complexity =< 10
	gocyclo -over 11 ./pkg
	# Run randomized, parallelized, racing, code coveraged, tests
	ginkgo -r \
		-cover -coverprofile=coverage.out -outputdir=. -coverpkg=./pkg/... \
		--randomizeAllSpecs --randomizeSuites -race
	go tool cover -html coverage.out -o coverage.html

verify: ## Verify code. Includes dependencies, linting, formatting, etc
	hack/boilerplate.sh
	go mod tidy
	go mod download
	go vet ./...
	go fmt ./...
	golangci-lint run --timeout 5m

licenses: ## Verifies dependency licenses and requires GITHUB_TOKEN to be set
	golicense hack/license-config.hcl bin/operator

apply: ## Apply the controller into your ~/.kube/config cluster
	helm template --include-crds kit charts/kit-operator --namespace $(KIT_NAMESPACE) \
		$(HELM_OPTS) \
		--set controller.image=ko://github.com/awslabs/kubernetes-iteration-toolkit/operator/cmd/controller \
		--set webhook.image=ko://github.com/awslabs/kubernetes-iteration-toolkit/operator/cmd/webhook \
		| KO_DOCKER_REPO=$(CONTAINER_IMAGE_REGISTRY)/kit $(WITH_GOFLAGS) ko apply --platform=linux/arm64,linux/amd64 --bare -f -
    
delete: ## Delete the controller from your ~/.kube/config cluster
	helm template kit charts/kit-operator --namespace $(KIT_NAMESPACE) \
		$(HELM_OPTS) \
		| kubectl delete -f -

codegen: ## Generate code. Must be run if changes are made to ./pkg/apis/...
	hack/codegen.sh
	hack/boilerplate.sh

docs:
	gen-crd-api-reference-docs \
		-api-dir ./pkg/apis/infrastructure/v1alpha1 \
		-config $(shell go env GOMODCACHE)/github.com/ahmetb/gen-crd-api-reference-docs@v0.3.0/example-config.json \
		-out-file docs/README.md \
		-template-dir $(shell go env GOMODCACHE)/github.com/ahmetb/gen-crd-api-reference-docs@v0.3.0/template

publish: ## Generate release manifests and publish a versioned container image.
	$(eval CONTROLLER_IMAGE := $(shell KO_DOCKER_REPO=public.ecr.aws/kit/kit-operator ko publish --platform=linux/arm64,linux/amd64 --bare -t $(RELEASE_VERSION) ./cmd/controller))
	@echo 'Controller image = '$(CONTROLLER_IMAGE)
	@test -n "$(CONTROLLER_IMAGE)" || (echo 'Controller Image version not defined' && exit 1)
	yq e -i ".controller.image = \""$(CONTROLLER_IMAGE)"\"" charts/kit-operator/values.yaml || { echo "failed"; exit 1; }

	$(eval WEBHOOK_IMAGE := $(shell KO_DOCKER_REPO=public.ecr.aws/kit/kit-webhook ko publish --platform=linux/arm64,linux/amd64 --bare -t $(RELEASE_VERSION) ./cmd/webhook))
	@echo 'Webhook image = '$(WEBHOOK_IMAGE)
	@test -n "$(WEBHOOK_IMAGE)" || (echo 'Webhook Image version not defined' && exit 1)
	yq e -i ".webhook.image = \""$(WEBHOOK_IMAGE)"\"" charts/kit-operator/values.yaml || { echo "failed"; exit 1; }

	yq e -i '.version = "$(subst v,,${RELEASE_VERSION})"' charts/kit-operator/Chart.yaml || { echo "failed"; exit 1; }

toolchain: ## Install developer toolchain
	./hack/toolchain.sh

.PHONY: help dev ci release test battletest verify codegen apply delete publish helm toolchain licenses deploy build
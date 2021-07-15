RELEASE_REPO ?= public.ecr.aws/i7d3g4r4/kit-operator
RELEASE_VERSION ?= $(shell git describe --tags --always)

## Inject the app version into project.Version
LDFLAGS ?= "-ldflags=-X=github.com/awslabs/kit/operator/pkg/utils/project.Version=$(RELEASE_VERSION)"
GOFLAGS ?= "$(LDFLAGS)"
WITH_GOFLAGS = GOFLAGS=$(GOFLAGS)
WITH_RELEASE_REPO = KO_DOCKER_REPO=$(RELEASE_REPO)
KIT_NAMESPACE ?= kit
CONTAINER_IMAGE_REGISTRY ?= 
## Extra helm options
HELM_OPTS ?=

help: ## Display help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

dev: verify test ## Run all steps in the developer loop

ci: verify licenses battletest ## Run all steps used by continuous integration

release: publish helm docs ## Run all steps in release workflow

test: ## Run tests
	ginkgo -r

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
	go build $(GOFLAGS) -o bin/operator cmd/controller/main.go
	golicense hack/license-config.hcl bin/operator

apply: ## Deploy the controller into your ~/.kube/config cluster
	kubectl create ns $(KIT_NAMESPACE) || true
	kubectl -n $(KIT_NAMESPACE) apply -f config/

deploy: 
	KO_DOCKER_REPO=$(CONTAINER_IMAGE_REGISTRY)/kit ko apply --bare -f config/
    
delete: ## Delete the controller from your ~/.kube/config cluster
	kubectl delete -f config
	kubectl -n $(KIT_NAMESPACE) delete ns $(KIT_NAMESPACE)

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
	@aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin $(RELEASE_REPO)
	KO_DOCKER_REPO=$(WITH_RELEASE_REPO) ko publish --bare ./cmd/controller

toolchain: ## Install developer toolchain
	./hack/toolchain.sh

.PHONY: help dev ci release test battletest verify codegen apply delete publish helm toolchain licenses deploy
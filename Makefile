# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.28

# Docker image name for the mkdocs based local development setup
MKDOCS_IMG=onmetal/libvirt-provider-docs

# Code depend on OS
TARGET_OS ?= linux
TARGET_ARCH ?= amd64
CGO_ENABLED ?= 1

LIBVIRT_PROVIDER_BIN=$(LOCALBIN)/libvirt-provider
LIBVIRT_PROVIDER_BIN_SOURCE=./cmd/libvirt-provider

GITHUB_PAT_PATH ?=
ifeq (,$(GITHUB_PAT_PATH))
GITHUB_PAT_MOUNT ?=
else
GITHUB_PAT_MOUNT ?= --secret id=github_pat,src=$(GITHUB_PAT_PATH)
endif

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

CONTAINER_BUILDARGS ?= --platform $(TARGET_OS)/$(TARGET_ARCH)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./..." output:rbac:artifacts:config=config/rbac

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: add-license
add-license: addlicense ## Add license headers to all go files.
	find . -name '*.go' -exec $(ADDLICENSE) -f hack/license-header.txt {} +

.PHONY: check-license
check-license: addlicense ## Check that every file has a license header present.
	find . -name '*.go' -exec $(ADDLICENSE) -check -c 'IronCore authors' {} +

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	GOOS=$(TARGET_OS) CGO_ENABLED=$(CGO_ENABLED) $(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	GOOS=$(TARGET_OS) CGO_ENABLED=$(CGO_ENABLED) $(GOLANGCI_LINT) run --fix

.PHONY: check
check: manifests generate fmt check-license lint test ## Generate manifests, code, lint, check licenses, test

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests. Some test depend on Linux OS
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go run github.com/onsi/ginkgo/v2/ginkgo run -r --label-filter="!integration" -coverprofile cover.out

.PHONY: integration-tests
integration-tests: ## Run integration tests against code. For dependencies, refer to the integration-test workflow.
	go run github.com/onsi/ginkgo/v2/ginkgo run -r --label-filter="integration" -coverprofile cover.out

##@ Documentation

.PHONY: start-docs
start-docs: ## Start the local mkdocs based development environment.
	$(CONTAINER_TOOL) build -t ${MKDOCS_IMG} -f docs/Dockerfile .
	$(CONTAINER_TOOL) run -p 8000:8000 -v `pwd`/:/docs ${MKDOCS_IMG}

.PHONY: clean-docs
clean-docs: ## Remove all local mkdocs Docker images (cleanup).
	$(CONTAINER_TOOL) container prune --force --filter "label=project=libvirt-provider_documentation"

##@ Build

.PHONY: build
build: manifests generate fmt vet add-license lint ## Build the binary
	GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o $(LIBVIRT_PROVIDER_BIN) $(LIBVIRT_PROVIDER_BIN_SOURCE)

.PHONY: run
run: manifests generate fmt vet ## Run the binary
	go run $(LIBVIRT_PROVIDER_BIN_SOURCE)/main.go \
		--supported-machine-classes config/development/machineclasses.json \
		--network-interface-plugin-name isolated \
		--address iri.sock

.PHONY: docker-build
docker-build: ## Build docker image with partitionlet
	$(CONTAINER_TOOL) build $(CONTAINER_BUILDARGS) -t ${IMG} $(GITHUB_PAT_MOUNT) .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: deploy
deploy: kustomize ## Deploy libvirt-provider into the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy libvirt-provider from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
ADDLICENSE ?= $(LOCALBIN)/addlicense-$(ADDLICENSE_VERSION)

## Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.14.0
ENVTEST_VERSION ?= release-0.16
GOLANGCI_LINT_VERSION ?= v1.57.2
ADDLICENSE_VERSION ?= v1.1.1

.PHONY: kustomize
kustomize: $(LOCALBIN) ## Download kustomize locally if necessary.
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(LOCALBIN) ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,${CONTROLLER_TOOLS_VERSION})

.PHONY: envtest
envtest: $(LOCALBIN) ## Download setup-envtest locally if necessary.
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,${ENVTEST_VERSION})

.PHONY: golangci-lint
golangci-lint: $(LOCALBIN) ## Download golangci-lint locally if necessary.
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

.PHONY: clean-tools
clean-tools: ## Clean any artifacts that can be regenerated.
	rm -rf $(LOCALBIN)

.PHONY: addlicense
addlicense: $(LOCALBIN) ## Download addlicense locally if necessary.
	$(call go-install-tool,$(ADDLICENSE),github.com/google/addlicense,${ADDLICENSE_VERSION})

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef

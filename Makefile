
# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.28

# Docker image name for the mkdocs based local development setup
MKDOCS_IMG=onmetal/libvirt-provider-docs

# Code depend on OS
TARGET_OS ?= linux
TARGET_ARCH ?= amd64

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

GITHUB_PAT_PATH ?=
ifeq (,$(GITHUB_PAT_PATH))
GITHUB_PAT_MOUNT ?=
else
GITHUB_PAT_MOUNT ?= --secret id=github_pat,src=$(GITHUB_PAT_PATH)
endif
DOCKER_BUILDARGS ?= --platform $(TARGET_OS)/$(TARGET_ARCH)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
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

.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	GOOS=$(TARGET_OS) CGO=1 $(GOLANGCI_LINT) run ./...

.PHONY: check
check: manifests generate add-license lint test

ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
.PHONY: test
test: manifests generate fmt envtest check-license ## Run tests. Some test depend on Linux OS
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

.PHONY: check
check: generate fmt add-license lint test ## Lint and run tests.

##@ Documentation

.PHONY: start-docs
start-docs: ## Start the local mkdocs based development environment.
	docker build -t ${MKDOCS_IMG} -f docs/Dockerfile .
	docker run -p 8000:8000 -v `pwd`/:/docs ${MKDOCS_IMG}

.PHONY: clean-docs
clean-docs: ## Remove all local mkdocs Docker images (cleanup).
	docker container prune --force --filter "label=project=virtlet_documentation"

##@ Build

.PHONY: build
build: generate fmt add-license lint ## Build the binary
	GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) go build -o bin/virtlet ./main.go

.PHONY: run
run-base: generate fmt lint ## Run the binary
	go run ./main.go

.PHONY: docker-build
docker-build: test ## Build docker image with partitionlet.
	docker build $(DOCKER_BUILDARGS) -t ${IMG} $(GITHUB_PAT_MOUNT) .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

##@ Tools

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_GEN_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
ADDLICENSE ?= $(LOCALBIN)/addlicense

## Tool Versions
KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_GEN_VERSION ?= v0.9.0
ENVTEST_VERSION ?= release-0.15
GOLANGCI_LINT_VERSION ?= v1.55.2
ADDLICENSE_VERSION ?= v1.1.1

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(LOCALBIN) ## Download kustomize locally if necessary.
	test -s $(KUSTOMIZE) || \
	(curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN) && \
	mv $(LOCALBIN)/kustomize $(KUSTOMIZE) )

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(CONTROLLER_GEN) || true
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,${CONTROLLER_GEN_VERSION})

.PHONY: envtest
envtest: $(LOCALBIN) ## Download envtest-setup locally if necessary.
	test -s $(ENVTEST) || true
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,${ENVTEST_VERSION})

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint || GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: addlicense
addlicense: $(ADDLICENSE) ## Download addlicense locally if necessary.
$(ADDLICENSE): $(LOCALBIN)
	test -s $(LOCALBIN)/addlicense || GOBIN=$(LOCALBIN) go install github.com/google/addlicense@$(ADDLICENSE_VERSION)


# go-install-tool will 'go install' any $1 package.
# it will add v. prefix into url, if url isn't contain version
define go-install-tool
@[ -f $(1) ] || { \
version=@$(3) ;\
url=$(2) ;\
if echo $$version | grep 'v[2-9][0-9]*' -q; then \
        echo $$url | grep '/v[2-9][0-9]*/' -q || version="/$$(printf $$version | grep -o 'v[2-9][0-9]*')$$version" ;\
fi ;\
echo "Downloading $${url}$${version}" ;\
GOBIN=$(GOBIN) go install $${url}$${version} ;\
binary=$$(echo "$$url" | rev | cut -d'/' -f 1 | rev) ;\
mv "$(GOBIN)/$${binary}" $(1) ;\
}
endef

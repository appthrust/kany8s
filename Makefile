# Image URL to use all building/pushing image targets
IMG ?= controller:latest
EKS_PLUGIN_IMG ?= eks-kubeconfig-rotator:latest
EKS_KARPENTER_BOOTSTRAPPER_IMG ?= eks-karpenter-bootstrapper:latest

# clusterctl provider name. Drives the directory names under out/ and the
# provider labels on the packaged kustomize resources.
CLUSTERCTL_NAME ?= kany8s
# Version used inside the generated metadata.yaml / layout. The release
# workflow keeps this at v0.0.0 because clusterctl only reads the actual
# release tag from the GitHub Release download URL, not from the embedded
# placeholder.
CLUSTERCTL_PROVIDER_VERSION ?= v0.0.0

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

# controller-gen scans all packages under the given paths.
# Keep refs/ out of generation to avoid pulling RBAC markers from reference code.
CONTROLLER_GEN_MANIFESTS_PATHS ?= paths="./api/..." paths="./internal/..."
CONTROLLER_GEN_GENERATE_PATHS ?= paths="./api/..."

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
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook $(CONTROLLER_GEN_MANIFESTS_PATHS) output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac
	# Append a v1beta2 served alias to every generated CRD so CAPI v1.13 +
	# Sveltos v1.8 can resolve our resources via API discovery in addition to
	# the CRD label hint that the topology controller follows. v1alpha1 stays
	# the storage version, conversion strategy stays the default (None), and
	# the schema is mirrored verbatim. See APTH-1563 + hack/add-v1beta2-alias.py.
	python3 hack/add-v1beta2-alias.py config/crd/bases/*.yaml

.PHONY: clusterapi-manifests
clusterapi-manifests: controller-gen ## Generate group-scoped CRDs and RBAC roles for the clusterctl provider bundle.
	mkdir -p config/clusterapi/infrastructure/bases
	mkdir -p config/clusterapi/controlplane/bases
	mkdir -p config/rbac
	# group-scoped RBAC (controlplane spans internal/controller + internal/controller/controlplane)
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role-infrastructure \
	    paths="./internal/controller/infrastructure/..." \
	    output:stdout > config/rbac/infrastructure-role.yaml
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role-controlplane \
	    paths="./internal/controller" \
	    paths="./internal/controller/controlplane/..." \
	    output:stdout > config/rbac/controlplane-role.yaml
	# group-scoped CRDs
	"$(CONTROLLER_GEN)" crd:generateEmbeddedObjectMeta=true webhook \
	    paths="./api/infrastructure/..." \
	    output:crd:artifacts:config=config/clusterapi/infrastructure/bases
	"$(CONTROLLER_GEN)" crd:generateEmbeddedObjectMeta=true webhook \
	    paths="./api/v1alpha1/..." \
	    output:crd:artifacts:config=config/clusterapi/controlplane/bases
	# Mirror the v1beta2 alias post-process onto the clusterctl bundle CRDs.
	python3 hack/add-v1beta2-alias.py \
	    config/clusterapi/infrastructure/bases/*.yaml \
	    config/clusterapi/controlplane/bases/*.yaml

.PHONY: clusterctl-setup
clusterctl-setup: clusterapi-manifests kustomize ## Render the clusterctl provider bundle (infrastructure + control-plane components + metadata) into out/.
	mkdir -p out/infrastructure-$(CLUSTERCTL_NAME)/$(CLUSTERCTL_PROVIDER_VERSION)
	mkdir -p out/control-plane-$(CLUSTERCTL_NAME)/$(CLUSTERCTL_PROVIDER_VERSION)
	# Swap the placeholder "controller" image in config/manager/kustomization.yaml
	# for the requested image. git restore below pins the working tree back.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=$(IMG)
	"$(KUSTOMIZE)" build --load-restrictor LoadRestrictionsNone \
	    config/clusterctl/infrastructure \
	    > out/infrastructure-$(CLUSTERCTL_NAME)/$(CLUSTERCTL_PROVIDER_VERSION)/infrastructure-components.yaml
	"$(KUSTOMIZE)" build --load-restrictor LoadRestrictionsNone \
	    config/clusterctl/controlplane \
	    > out/control-plane-$(CLUSTERCTL_NAME)/$(CLUSTERCTL_PROVIDER_VERSION)/control-plane-components.yaml
	git restore config/manager/kustomization.yaml
	cp hack/capi/metadata.yaml out/infrastructure-$(CLUSTERCTL_NAME)/$(CLUSTERCTL_PROVIDER_VERSION)/metadata.yaml
	cp hack/capi/metadata.yaml out/control-plane-$(CLUSTERCTL_NAME)/$(CLUSTERCTL_PROVIDER_VERSION)/metadata.yaml
	# Render a local clusterctl config that points at the generated files so
	# smoke tests can run `clusterctl init --config capi-local-config.yaml`
	# without publishing a GitHub Release first. Substitute both the working
	# directory and the provider version so CLUSTERCTL_PROVIDER_VERSION
	# overrides flow through end-to-end.
	sed -e 's#%pwd%#'`pwd`'#g' \
	    -e 's#%version%#$(CLUSTERCTL_PROVIDER_VERSION)#g' \
	    ./hack/capi/config.yaml > capi-local-config.yaml

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" $(CONTROLLER_GEN_GENERATE_PATHS)

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= kany8s-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: test-acceptance-kro-reflection
test-acceptance-kro-reflection: ## Run acceptance test script (kind + kro + Kany8sControlPlane status reflection).
	bash hack/acceptance-test-kro-reflection.sh

.PHONY: test-acceptance-kro-reflection-keep
test-acceptance-kro-reflection-keep: ## Run kro reflection acceptance test and keep the kind cluster.
	CLEANUP=false bash hack/acceptance-test-kro-reflection.sh

.PHONY: test-acceptance-kro-infra-reflection
test-acceptance-kro-infra-reflection: ## Run acceptance test script (kind + kro + Kany8sCluster infra reflection).
	bash hack/acceptance-test-kro-infra-reflection.sh

.PHONY: test-acceptance-kro-infra-reflection-keep
test-acceptance-kro-infra-reflection-keep: ## Run kro infra reflection acceptance test and keep the kind cluster.
	CLEANUP=false bash hack/acceptance-test-kro-infra-reflection.sh

.PHONY: test-acceptance-kro-infra-cluster-identity
test-acceptance-kro-infra-cluster-identity: ## Run kro infra cluster identity acceptance test (clusterUID injection + ownerRef/label).
	bash hack/acceptance-test-kro-infra-cluster-identity.sh

.PHONY: test-acceptance-kro-infra-cluster-identity-keep
test-acceptance-kro-infra-cluster-identity-keep: ## Run kro infra cluster identity acceptance test and keep the kind cluster.
	CLEANUP=false bash hack/acceptance-test-kro-infra-cluster-identity.sh

.PHONY: test-acceptance-capd-kubeadm
test-acceptance-capd-kubeadm: ## Run acceptance test script (kind + clusterctl + CAPD + kubeadm).
	bash hack/acceptance-test-capd-kubeadm.sh

.PHONY: test-acceptance-capd-kubeadm-keep
test-acceptance-capd-kubeadm-keep: ## Run CAPD+kubeadm acceptance test and keep the kind cluster.
	CLEANUP=false bash hack/acceptance-test-capd-kubeadm.sh

.PHONY: test-acceptance-kro-reflection-multi-rgd
test-acceptance-kro-reflection-multi-rgd: ## Run kro reflection acceptance test with multiple RGDs.
	bash hack/acceptance-test-kro-reflection-multi-rgd.sh

.PHONY: test-acceptance-kro-reflection-multi-rgd-keep
test-acceptance-kro-reflection-multi-rgd-keep: ## Run kro reflection multi-RGD acceptance test and keep the kind cluster.
	CLEANUP=false bash hack/acceptance-test-kro-reflection-multi-rgd.sh

# Legacy aliases (kept for compatibility)
.PHONY: test-acceptance
test-acceptance: test-acceptance-kro-reflection ## Legacy alias for kro reflection.

.PHONY: test-acceptance-keep
test-acceptance-keep: test-acceptance-kro-reflection-keep ## Legacy alias for kro reflection (keep cluster).

.PHONY: test-acceptance-self-managed
test-acceptance-self-managed: test-acceptance-capd-kubeadm ## Legacy alias for CAPD+kubeadm.

.PHONY: test-acceptance-self-managed-keep
test-acceptance-self-managed-keep: test-acceptance-capd-kubeadm-keep ## Legacy alias for CAPD+kubeadm (keep cluster).

.PHONY: test-acceptance-multi-rgd
test-acceptance-multi-rgd: test-acceptance-kro-reflection-multi-rgd ## Legacy alias for kro reflection (multi RGD).

.PHONY: test-acceptance-multi-rgd-keep
test-acceptance-multi-rgd-keep: test-acceptance-kro-reflection-multi-rgd-keep ## Legacy alias for kro reflection (multi RGD; keep cluster).

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: build-eks-plugin
build-eks-plugin: manifests generate fmt vet ## Build EKS kubeconfig rotator binary.
	go build -o bin/eks-kubeconfig-rotator cmd/eks-kubeconfig-rotator/main.go

.PHONY: build-eks-karpenter-bootstrapper
build-eks-karpenter-bootstrapper: manifests generate fmt vet ## Build EKS Karpenter bootstrapper binary.
	go build -o bin/eks-karpenter-bootstrapper cmd/eks-karpenter-bootstrapper/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: run-eks-plugin
run-eks-plugin: manifests generate fmt vet ## Run EKS kubeconfig rotator from your host.
	go run ./cmd/eks-kubeconfig-rotator/main.go

.PHONY: run-eks-karpenter-bootstrapper
run-eks-karpenter-bootstrapper: manifests generate fmt vet ## Run EKS Karpenter bootstrapper from your host.
	go run ./cmd/eks-karpenter-bootstrapper/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-build-eks-plugin
docker-build-eks-plugin: ## Build docker image with EKS kubeconfig rotator.
	$(CONTAINER_TOOL) build -f Dockerfile.eks-plugin -t ${EKS_PLUGIN_IMG} .

.PHONY: docker-build-eks-karpenter-bootstrapper
docker-build-eks-karpenter-bootstrapper: ## Build docker image with EKS Karpenter bootstrapper.
	$(CONTAINER_TOOL) build -f Dockerfile.eks-karpenter-bootstrapper -t ${EKS_KARPENTER_BOOTSTRAPPER_IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-push-eks-plugin
docker-push-eks-plugin: ## Push docker image with EKS kubeconfig rotator.
	$(CONTAINER_TOOL) push ${EKS_PLUGIN_IMG}

.PHONY: docker-push-eks-karpenter-bootstrapper
docker-push-eks-karpenter-bootstrapper: ## Push docker image with EKS Karpenter bootstrapper.
	$(CONTAINER_TOOL) push ${EKS_KARPENTER_BOOTSTRAPPER_IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name kany8s-builder
	$(CONTAINER_TOOL) buildx use kany8s-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm kany8s-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy-eks-plugin
deploy-eks-plugin: kustomize ## Deploy EKS kubeconfig rotator to the K8s cluster specified in ~/.kube/config.
	cd config/eks-plugin && "$(KUSTOMIZE)" edit set image example.com/eks-kubeconfig-rotator=${EKS_PLUGIN_IMG}
	"$(KUSTOMIZE)" build config/eks-plugin | "$(KUBECTL)" apply -f -

.PHONY: deploy-eks-karpenter-bootstrapper
deploy-eks-karpenter-bootstrapper: kustomize ## Deploy EKS Karpenter bootstrapper to the K8s cluster specified in ~/.kube/config.
	cd config/eks-karpenter-bootstrapper && "$(KUSTOMIZE)" edit set image example.com/eks-karpenter-bootstrapper=${EKS_KARPENTER_BOOTSTRAPPER_IMG}
	"$(KUSTOMIZE)" build config/eks-karpenter-bootstrapper | "$(KUBECTL)" apply -f -

.PHONY: undeploy-eks-plugin
undeploy-eks-plugin: kustomize ## Undeploy EKS kubeconfig rotator from the K8s cluster specified in ~/.kube/config.
	"$(KUSTOMIZE)" build config/eks-plugin | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: undeploy-eks-karpenter-bootstrapper
undeploy-eks-karpenter-bootstrapper: kustomize ## Undeploy EKS Karpenter bootstrapper from the K8s cluster specified in ~/.kube/config.
	"$(KUSTOMIZE)" build config/eks-karpenter-bootstrapper | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

##@ Helm Charts

# Destination for `helm package` output. Cleaned by `make clean-helm`.
HELM_DIST ?= dist/charts
# Local OCI registry used by helm-push-local. Matches the default Zot port
# from /srv/platform/CLAUDE.md "Assets Architecture". Override with e.g.
# HELM_LOCAL_REGISTRY=oci://localhost:5099/charts for a non-standard port.
HELM_LOCAL_REGISTRY ?= oci://localhost:5001/charts

.PHONY: helm-lint
helm-lint: ## Lint every chart under charts/; fails non-zero on any error.
	@set -e; for chart in charts/*/; do \
		echo ">>> helm lint $$chart"; \
		helm lint "$$chart"; \
	done

.PHONY: helm-package
helm-package: ## Package every chart under charts/ into $(HELM_DIST).
	@mkdir -p "$(HELM_DIST)"
	@set -e; for chart in charts/*/; do \
		echo ">>> helm package $$chart"; \
		helm package "$$chart" --destination "$(HELM_DIST)"; \
	done

.PHONY: helm-push-local
helm-push-local: helm-package ## Push packaged charts to the local Zot registry at $(HELM_LOCAL_REGISTRY).
	@set -e; for pkg in $(HELM_DIST)/*.tgz; do \
		echo ">>> helm push $$pkg $(HELM_LOCAL_REGISTRY)"; \
		helm push "$$pkg" "$(HELM_LOCAL_REGISTRY)"; \
	done

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.7.1
CONTROLLER_TOOLS_VERSION ?= v0.20.0

# ENVTEST_VERSION is the version of setup-envtest to install.
# Keep this pinned via go.mod for reproducible test environments.
ENVTEST_VERSION ?= $(call gomodver,sigs.k8s.io/controller-runtime/tools/setup-envtest)

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

GOLANGCI_LINT_VERSION ?= v2.7.2
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef

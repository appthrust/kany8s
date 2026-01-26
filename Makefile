SHELL := /usr/bin/env bash

LOCALBIN ?= $(shell pwd)/bin

CONTROLLER_TOOLS_VERSION ?= v0.17.0
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) crd:crdVersions=v1 paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)

$(CONTROLLER_GEN): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

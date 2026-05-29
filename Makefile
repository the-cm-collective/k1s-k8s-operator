.PHONY: all test vet generate manifests verify kustomize-build dry-run diff-check

GO ?= go
CONTROLLER_GEN ?= controller-gen
KUSTOMIZE ?= kustomize
KUBECTL ?= kubectl

LOCAL_GO := /tmp/go1.26.3.linux-amd64/go/bin/go
LOCAL_TOOLS := /tmp/k1s-operator-tools

ifneq ($(wildcard $(LOCAL_GO)),)
GO := $(LOCAL_GO)
export PATH := $(dir $(LOCAL_GO)):$(PATH)
endif

ifneq ($(wildcard $(LOCAL_TOOLS)/controller-gen),)
CONTROLLER_GEN := $(LOCAL_TOOLS)/controller-gen
endif
export PATH := $(LOCAL_TOOLS):$(PATH)

all: verify

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

generate:
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

manifests:
	$(CONTROLLER_GEN) \
		rbac:roleName=manager-role \
		crd \
		paths="./..." \
		output:crd:artifacts:config=config/crd/bases \
		output:rbac:artifacts:config=config/rbac

kustomize-build:
	$(KUSTOMIZE) build config/default >/tmp/k1s-operator-default.yaml
	$(KUSTOMIZE) build config/samples >/tmp/k1s-operator-samples.yaml

dry-run: kustomize-build
	$(KUBECTL) apply --server-side --dry-run=server -f /tmp/k1s-operator-default.yaml

diff-check:
	git diff --check

verify: generate manifests test vet kustomize-build diff-check

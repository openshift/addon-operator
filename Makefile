SHELL=/usr/bin/env bash
.SHELLFLAGS=-euo pipefail -c

# Dependency Versions
CONTROLLER_GEN_VERSION:=v0.5.0
OLM_VERSION:=v0.17.0
KIND_VERSION:=v0.10.0
YQ_VERSION:=v4@v4.7.0
GOIMPORTS_VERSION:=v0.1.0
GOLANGCI_LINT_VERSION:=v1.39.0
OPM_VERSION:=v1.17.2

# Build Flags
export CGO_ENABLED:=0
BRANCH=$(shell git rev-parse --abbrev-ref HEAD)
SHORT_SHA=$(shell git rev-parse --short HEAD)
VERSION?=$(shell echo ${BRANCH} | tr / -)-${SHORT_SHA}
BUILD_DATE=$(shell date +%s)
MODULE:=github.com/openshift/addon-operator
LD_FLAGS=-X $(MODULE)/internal/version.Version=$(VERSION) \
			-X $(MODULE)/internal/version.Branch=$(BRANCH) \
			-X $(MODULE)/internal/version.Commit=$(SHORT_SHA) \
			-X $(MODULE)/internal/version.BuildDate=$(BUILD_DATE)

# PATH/Bin
DEPENDENCIES:=$(shell pwd)/.cache/dependencies
export GOBIN?=$(DEPENDENCIES)/bin
export PATH:=$(GOBIN):$(PATH)

# Config
KIND_KUBECONFIG_DIR:=.cache/e2e
KIND_KUBECONFIG:=$(KIND_KUBECONFIG_DIR)/kubeconfig
export KUBECONFIG?=$(abspath $(KIND_KUBECONFIG))
API_BASE:=addons.managed.openshift.io
export SKIP_TEARDOWN?=

# Container
IMAGE_ORG?=quay.io/app-sre
ADDON_OPERATOR_MANAGER_IMAGE?=$(IMAGE_ORG)/addon-operator-manager:$(VERSION)


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

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


##@ Compile

all: bin/linux_amd64/addon-operator-manager ## compile operator for amd64 arch

bin/linux_amd64/%: GOARGS = GOOS=linux GOARCH=amd64

bin/%: generate FORCE
	$(eval COMPONENT=$(shell basename $*))
	@echo -e -n "compiling cmd/$(COMPONENT)...\n  "
	$(GOARGS) go build -ldflags "-w $(LD_FLAGS)" -o bin/$* cmd/$(COMPONENT)/main.go
	@echo

FORCE:

# prints the version as used by build commands.
version: ## print version
	@echo $(VERSION)
.PHONY: version

clean: ## rm -fr bin .cache
	@rm -rf bin .cache
.PHONY: clean

##@ Dependencies

KIND := $(DEPENDENCIES)/bin/kind
kind: ## Download kind locally if necessary.
	$(call go-get-tool,$(KIND),sigs.k8s.io/kind@$(KIND_VERSION))

CONTROLLER_GEN := $(DEPENDENCIES)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION))

GOIMPORTS := $(DEPENDENCIES)/bin/goimports
goimports: ## Download goimports locally if necessary.
	$(call go-get-tool,$(GOIMPORTS),golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION))

YQ := $(DEPENDENCIES)/bin/yq
yq: ## Download yq locally if necessary.
	$(call go-get-tool,$(YQ),github.com/mikefarah/yq/$(YQ_VERSION))

GOLANGCI_LINT := $(DEPENDENCIES)/bin/golangci-lint
export GOLANGCI_LINT_CACHE=$(abspath .cache/golangci-lint)
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION))

# go-get-tool will 'go get' any package $2 and install it to $1. (from kubebuilder)
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(DEPENDENCIES)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

OPM:=$(DEPENDENCIES)/bin/opm
opm: ## Download operator-framework/operator-registry binary.
	@echo "installing opm $(OPM_VERSION)..."
	$(eval OPM_TMP := $(shell mktemp -d))
	@(cd "$(OPM_TMP)"; \
		curl -sfL \
		https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/linux-amd64-opm -o opm; \
		chmod +x opm; \
		mv opm $(DEPENDENCIES)/bin/; \
	) 2>&1 | sed 's/^/  /'
	@rm -rf "$(OPM_TMP)"

all-deps: kind controller-gen yq goimports golangci-lint opm ## Downloads all project deps.
.PHONY: all-deps kind controller-gen yq goimports golangci-lint opm

##@ Deployment


run: generate ## Run against the configured Kubernetes cluster in ~/.kube/config or $KUBECONFIG
	go run -ldflags "-w $(LD_FLAGS)" \
		./cmd/addon-operator-manager/main.go \
			-pprof-addr="127.0.0.1:8065"
.PHONY: run

##@ Generators

generate: $(CONTROLLER_GEN) ## Generate code and manifests e.g. CRD, RBAC etc.
	@echo "generating kubernetes manifests..."
	@controller-gen crd:crdVersions=v1 \
		rbac:roleName=addon-operator-manager \
		paths="./..." \
		output:crd:artifacts:config=config/deploy 2>&1 | sed 's/^/  /'
	@echo
	@echo "generating code..."
	@controller-gen object paths=./apis/... 2>&1 | sed 's/^/  /'
	@echo
.PHONY: generate

sandwich: ## Makes sandwich - https://xkcd.com/149/
ifneq ($(shell id -u), 0)
	@echo "What? Make it yourself."
else
	@echo "Okay."
endif
.PHONY: sandwich

##@ Testing and Linting

lint: generate $(GOLANGCI_LINT) ## Runs code-generators, checks for clean directory and lints the source code.
	go fmt ./...
	@hack/validate-directory-clean.sh
	golangci-lint run ./... --deadline=15m
.PHONY: lint

test-unit: generate ## Runs unittests
	CGO_ENABLED=1 go test -race -v ./internal/... ./cmd/...
.PHONY: test-unit

# FORCE_FLAGS ensures that the tests will not be cached
FORCE_FLAGS = -count=1
test-e2e: config/deploy/deployment.yaml ## Runs the E2E testsuite against the currently selected cluster.
	@echo "running e2e tests..."
	@go test -v $(FORCE_FLAGS) ./e2e/...
.PHONY: test-e2e


test-e2e-local: export KUBECONFIG=$(abspath $(KIND_KUBECONFIG))
test-e2e-local: | setup-e2e-kind test-e2e ## Sets up a local kind cluster and runs E2E tests against this local cluster.
.PHONY: test-e2e-local

test-e2e-ci: | apply-ao test-e2e ## Run the E2E testsuite after installing the AddonOperator into the cluster.
.PHONY: test-e2e-ci


setup-e2e-kind: export KUBECONFIG=$(abspath $(KIND_KUBECONFIG))
setup-e2e-kind: | create-kind-cluster apply-olm apply-openshift-console load-ao ## make sure that we install our components into the kind cluster and disregard normal $KUBECONFIG
.PHONY: setup-e2e-kind

create-kind-cluster: $(KIND) ## create kind cluster
	@echo "creating kind cluster addon-operator-e2e..."
	@mkdir -p .cache/e2e
	@(source hack/determine-container-runtime.sh; \
		mkdir -p $(KIND_KUBECONFIG_DIR); \
		$$KIND_COMMAND create cluster \
			--kubeconfig=$(KIND_KUBECONFIG) \
			--name="addon-operator-e2e"; \
		if [[ ! -O "$(KIND_KUBECONFIG)" ]]; then \
			sudo chown $$USER: "$(KIND_KUBECONFIG)"; \
		fi; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: create-kind-cluster

delete-kind-cluster: $(KIND) ## delete kind cluster
	@echo "deleting kind cluster addon-operator-e2e..."
	@(source hack/determine-container-runtime.sh; \
		$$KIND_COMMAND delete cluster \
			--kubeconfig="$(KIND_KUBECONFIG)" \
			--name "addon-operator-e2e"; \
		rm -rf "$(KIND_KUBECONFIG)"; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: delete-kind-cluster

apply-olm: ## Installs OLM (Operator Lifecycle Manager) into the currently selected cluster.
	@echo "installing OLM $(OLM_VERSION)..."
	@(kubectl apply -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/$(OLM_VERSION)/crds.yaml; \
		kubectl apply -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/$(OLM_VERSION)/olm.yaml; \
		echo -e "\nwaiting for deployment/olm-operator..."; \
		kubectl wait --for=condition=available deployment/olm-operator -n olm --timeout=240s; \
		echo -e "\nwaiting for deployment/catalog-operator..."; \
		kubectl wait --for=condition=available deployment/catalog-operator -n olm --timeout=240s; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: apply-olm

apply-openshift-console: ## Installs the OpenShift/OKD console into the currently selected cluster.
	@echo "installing OpenShift console :latest..."
	@(kubectl apply -f hack/openshift-console.yaml; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: apply-openshift-console

load-ao: build-image-addon-operator-manager ## Load Addon Operator Image into kind
	@source hack/determine-container-runtime.sh; \
		$$KIND_COMMAND load image-archive \
			.cache/image/addon-operator-manager.tar \
			--name addon-operator-e2e;
.PHONY: load-ao

config/deploy/deployment.yaml: FORCE $(YQ) ## Template deployment
	@yq eval '.spec.template.spec.containers[0].image = "$(ADDON_OPERATOR_MANAGER_IMAGE)"' \
		config/deploy/deployment.yaml.tpl > config/deploy/deployment.yaml

apply-ao: $(YQ) load-ao config/deploy/deployment.yaml ## Installs the Addon Operator into the kind e2e cluster.
	@echo "installing Addon Operator $(VERSION)..."
	@(source hack/determine-container-runtime.sh; \
		kubectl apply -f config/deploy; \
		echo -e "\nwaiting for deployment/addon-operator..."; \
		kubectl wait --for=condition=available deployment/addon-operator -n addon-operator --timeout=240s; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: apply-ao

##@ OLM


config/olm/addon-operator.csv.yaml: FORCE $(YQ) ## Template Cluster Service Version / CSV by setting the container image to deploy.
	@yq eval '.spec.install.spec.deployments[0].spec.template.spec.containers[0].image = "$(ADDON_OPERATOR_MANAGER_IMAGE)" | .metadata.annotations.containerImage = "$(ADDON_OPERATOR_MANAGER_IMAGE)"' \
	config/olm/addon-operator.csv.tpl.yaml > config/olm/addon-operator.csv.yaml

# The first few lines of the CRD file need to be removed:
# https://github.com/operator-framework/operator-registry/issues/222
build-image-addon-operator-bundle: clean-image-cache-addon-operator-bundle config/olm/addon-operator.csv.yaml ## Bundle image contains the manifests and CSV for a single version of this operator.
	$(eval IMAGE_NAME := addon-operator-bundle)
	@echo "building image ${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}..."
	@(source hack/determine-container-runtime.sh; \
		mkdir -p ".cache/image/${IMAGE_NAME}/manifests"; \
		mkdir -p ".cache/image/${IMAGE_NAME}/metadata"; \
		cp -a "config/olm/addon-operator.csv.yaml" ".cache/image/${IMAGE_NAME}/manifests"; \
		cp -a "config/olm/annotations.yaml" ".cache/image/${IMAGE_NAME}/metadata"; \
		cp -a "config/docker/${IMAGE_NAME}.Dockerfile" ".cache/image/${IMAGE_NAME}/Dockerfile"; \
		tail -n"+3" "config/deploy/addons.managed.openshift.io_addons.yaml" > ".cache/image/${IMAGE_NAME}/manifests/addons.crd.yaml"; \
		$$CONTAINER_COMMAND build -t "${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}" ".cache/image/${IMAGE_NAME}"; \
		$$CONTAINER_COMMAND image save -o ".cache/image/${IMAGE_NAME}.tar" "${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}"; \
		echo) 2>&1 | sed 's/^/  /'
.PHONY: build-image-addon-operator-bundle

# Warning!
# The bundle image needs to be pushed so the opm CLI can create the index image.
build-image-addon-operator-index: $(OPM) clean-image-cache-addon-operator-index | build-image-addon-operator-bundle push-image-addon-operator-bundle ## Index image contains a list of bundle images for use in a CatalogSource.
	$(eval IMAGE_NAME := addon-operator-index)
	@echo "building image ${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}..."
	@(source hack/determine-container-runtime.sh; \
		opm index add --container-tool $$CONTAINER_COMMAND \
		--bundles ${IMAGE_ORG}/addon-operator-bundle:${VERSION} \
		--tag ${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}; \
		$$CONTAINER_COMMAND image save -o ".cache/image/${IMAGE_NAME}.tar" "${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}"; \
		echo) 2>&1 | sed 's/^/  /'
.PHONY: build-image-addon-operator-index

##@ Container Images

build-images: build-image-addon-operator-manager ## build all images
.PHONY: build-images

push-images: push-image-addon-operator-manager ## push all images
.PHONY: push-images

.SECONDEXPANSION:
# cleans the built image .tar and image build directory
clean-image-cache-%:
	@rm -rf ".cache/image/$*" ".cache/image/$*.tar"
	@mkdir -p ".cache/image/$*"

build-image-%: bin/linux_amd64/$$*
	@echo "building image ${IMAGE_ORG}/$*:${VERSION}..."
	@(source hack/determine-container-runtime.sh; \
		rm -rf ".cache/image/$*" ".cache/image/$*.tar"; \
		mkdir -p ".cache/image/$*"; \
		cp -a "bin/linux_amd64/$*" ".cache/image/$*"; \
		cp -a "config/docker/$*.Dockerfile" ".cache/image/$*/Dockerfile"; \
		cp -a "config/docker/passwd" ".cache/image/$*/passwd"; \
		$$CONTAINER_COMMAND build -t "${IMAGE_ORG}/$*:${VERSION}" ".cache/image/$*"; \
		$$CONTAINER_COMMAND image save -o ".cache/image/$*.tar" "${IMAGE_ORG}/$*:${VERSION}"; \
		echo; \
	) 2>&1 | sed 's/^/  /'

push-image-%: build-image-$$*
	@echo "pushing image ${IMAGE_ORG}/$*:${VERSION}..."
	@(source hack/determine-container-runtime.sh; \
		$$CONTAINER_COMMAND push "${IMAGE_ORG}/$*:${VERSION}"; \
		echo pushed "${IMAGE_ORG}/$*:${VERSION}"; \
		echo; \
	) 2>&1 | sed 's/^/  /'

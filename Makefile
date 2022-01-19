SHELL=/bin/bash
.SHELLFLAGS=-euo pipefail -c

# Dependency Versions
CONTROLLER_GEN_VERSION:=v0.6.2
OLM_VERSION:=v0.19.1
KIND_VERSION:=v0.11.1
YQ_VERSION:=v4@v4.12.0
GOIMPORTS_VERSION:=v0.1.5
GOLANGCI_LINT_VERSION:=v1.43.0
OPM_VERSION:=v1.18.0

# Build Flags
export CGO_ENABLED:=0
BRANCH=$(shell git rev-parse --abbrev-ref HEAD)
SHORT_SHA=$(shell git rev-parse --short HEAD)
VERSION?=${SHORT_SHA}
BUILD_DATE=$(shell date +%s)
MODULE:=github.com/openshift/addon-operator
GOFLAGS=
LD_FLAGS=-X $(MODULE)/internal/version.Version=$(VERSION) \
			-X $(MODULE)/internal/version.Branch=$(BRANCH) \
			-X $(MODULE)/internal/version.Commit=$(SHORT_SHA) \
			-X $(MODULE)/internal/version.BuildDate=$(BUILD_DATE)

UNAME_OS:=$(shell uname -s)
UNAME_OS_LOWER:=$(shell uname -s | awk '{ print tolower($$0); }') # UNAME_OS but in lower case
UNAME_ARCH:=$(shell uname -m)

# PATH/Bin
PROJECT_DIR:=$(shell pwd)
DEPENDENCIES:=.cache/dependencies
DEPENDENCY_BIN:=$(abspath $(DEPENDENCIES)/bin)
DEPENDENCY_VERSIONS:=$(abspath $(DEPENDENCIES)/$(UNAME_OS)/$(UNAME_ARCH)/versions)
export PATH:=$(DEPENDENCY_BIN):$(PATH)

# Config
KIND_KUBECONFIG_DIR:=.cache/integration
KIND_KUBECONFIG:=$(KIND_KUBECONFIG_DIR)/kubeconfig
export KUBECONFIG?=$(abspath $(KIND_KUBECONFIG))
export GOLANGCI_LINT_CACHE=$(abspath .cache/golangci-lint)
export SKIP_TEARDOWN?=
KIND_CLUSTER_NAME:="addon-operator" # name of the kind cluster for local development.
ENABLE_API_MOCK?="false"
ENABLE_WEBHOOK?="false"
ENABLE_MONITORING?="true"
WEBHOOK_PORT?=8080

# Container
IMAGE_ORG?=quay.io/app-sre
ADDON_OPERATOR_MANAGER_IMAGE?=$(IMAGE_ORG)/addon-operator-manager:$(VERSION)
ADDON_OPERATOR_WEBHOOK_IMAGE?=$(IMAGE_ORG)/addon-operator-webhook:$(VERSION)
API_MOCK_IMAGE?=$(IMAGE_ORG)/api-mock:$(VERSION)

# COLORS
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
RESET  := $(shell tput -Txterm sgr0)

# ---------
##@ General
# ---------

# Default build target - must be first!
all: \
	bin/linux_amd64/addon-operator-manager \
	bin/linux_amd64/addon-operator-webhook \
	bin/linux_amd64/api-mock

## Display this help.
help:
	@echo 'Usage:'
	@echo '  ${YELLOW}make${RESET} ${GREEN}<target>${RESET}'
	@awk \
	'/^[^[:space:]]+:/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			printf "  ${GREEN}%-30s${RESET}%s\n", helpCommand, helpMessage; \
		} \
	} \
	/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

## Prints version as used by build commands.
version:
	@echo $(VERSION)
.PHONY: version

## Cleans cached binaries, dependencies and container image tars.
clean: delete-kind-cluster
	@rm -rf bin .cache
.PHONY: clean

# ---------
##@ Compile
# ---------

## Forces GOOS=linux GOARCH=amd64. For bin/%.
bin/linux_amd64/%: GOARGS = GOOS=linux GOARCH=amd64

## Builds binaries from cmd/%.
bin/%: generate FORCE
	$(eval COMPONENT=$(shell basename $*))
	@echo -e -n "compiling cmd/$(COMPONENT)...\n  "
	$(GOARGS) go build -ldflags "-w $(LD_FLAGS)" -o bin/$* cmd/$(COMPONENT)/main.go
	@echo

# empty force target to ensure a target always executes.
FORCE:

# ----------------------------
# Dependencies (project local)
# ----------------------------

# go-install-tool will 'go install' any package $1 if file $2 does not exist.
define go-install-tool
@[ -f "$(2)" ] || { \
	TMP_DIR=$$(mktemp -d); \
	cd $$TMP_DIR; \
	go mod init tmp; \
	echo "Downloading $(1) to $(DEPENDENCIES)/bin"; \
	GOBIN="$(DEPENDENCY_BIN)" go install -mod=readonly "$(1)"; \
	rm -rf $$TMP_DIR; \
	mkdir -p "$(dir $(2))"; \
	touch "$(2)"; \
}
endef

KIND:=$(DEPENDENCY_VERSIONS)/kind/$(KIND_VERSION)
$(KIND):
	@$(call go-install-tool,sigs.k8s.io/kind@$(KIND_VERSION),$(KIND))
	@(command -v kind; kind version) | sed 's/^/  /'

CONTROLLER_GEN:=$(DEPENDENCY_VERSIONS)/controller-gen/$(CONTROLLER_GEN_VERSION)
$(CONTROLLER_GEN):
	@$(call go-install-tool,sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION),$(CONTROLLER_GEN))
	@(echo; command -v controller-gen; controller-gen --version; echo) | sed 's/^/  /'

YQ:=$(DEPENDENCY_VERSIONS)/yq/$(YQ_VERSION)
$(YQ):
	@$(call go-install-tool,github.com/mikefarah/yq/$(YQ_VERSION),$(YQ))
	@(command -v yq; yq --version) | sed 's/^/  /'

GOIMPORTS:=$(DEPENDENCY_VERSIONS)/goimports/$(GOIMPORTS_VERSION)
$(GOIMPORTS):
	@$(call go-install-tool,golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION),$(GOIMPORTS))
	# goimports doesn't have a version flag
	@(echo; command -v goimports; echo) | sed 's/^/  /'

# Setup goimports.
# alias for goimports to use from `ensure-and-run-goimports.sh` via pre-commit.
goimports: $(GOIMPORTS)
.PHONY: goimports

GOLANGCI_LINT:=$(DEPENDENCY_VERSIONS)/golangci-lint/$(GOLANGCI_LINT_VERSION)
$(GOLANGCI_LINT):
	@$(call go-install-tool,github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION),$(GOLANGCI_LINT))
	@(echo; command -v golangci-lint; golangci-lint --version; echo) | sed 's/^/  /'

# Setup golangci-lint.
# alias for golangci-lint to use from `ensure-and-run-golangci-lint.sh` via pre-commit.
golangci-lint: $(GOLANGCI_LINT)
.PHONY: golangci-lint

OPM:=$(DEPENDENCY_VERSIONS)/opm/$(OPM_VERSION)
$(OPM):
	@echo "installing opm $(OPM_VERSION)..."
	$(eval OPM_TMP := $(shell mktemp -d))
	@(cd "$(OPM_TMP)"; \
		curl -L --fail \
		https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/linux-amd64-opm -o opm; \
		chmod +x opm; \
		mv opm $(DEPENDENCY_BIN); \
	) 2>&1 | sed 's/^/  /'
	@rm -rf "$(OPM_TMP)" "$(dir $(OPM))" \
		&& mkdir -p "$(dir $(OPM))" \
		&& touch "$(OPM)" \
		&& echo
	@(echo; command -v opm; opm version; echo) | sed 's/^/  /'

HELM:=$(DEPENDENCY_VERSIONS)/helm/$(HELM_VERSION)
$(HELM):
	@echo "installing helm ${HELM_VERSION}"
	$(eval HELM_TMP = $(shell mktemp -d))
	@(cd "$(HELM_TMP)"; \
		curl -L --fail \
		https://get.helm.sh/helm-$(HELM_VERSION)-$(UNAME_OS_LOWER)-amd64.tar.gz -o helm.tar.gz; \
		tar xvf helm.tar.gz; \
		cp $(UNAME_OS_LOWER)-amd64/helm helm; \
		chmod +x helm; \
		mv helm $(DEPENDENCY_BIN); \
	) 2>&1 | sed 's/^/  /'
	@rm -rf "$(HELM_TMP)" "$(dir $(HELM))" \
		&& mkdir -p "$(dir $(HELM))" \
		&& touch "$(HELM)" \
		&& echo
	@(echo; command -v helm; helm version; echo) | sed 's/^/  /'


## Run go mod tidy in all go modules
tidy:
	@./mage tidy
.PHONY: tidy

# ------------
##@ Generators
# ------------

## Generate deepcopy code, kubernetes manifests and docs.
generate:
	@./mage generate
.PHONY: generate

# Makes sandwich
# https://xkcd.com/149/
sandwich:
ifneq ($(shell id -u), 0)
	@echo "What? Make it yourself."
else
	@echo "Okay."
endif
.PHONY: sandwich

# ---------------------
##@ Testing and Linting
# ---------------------

## Runs code-generators, checks for clean directory and lints the source code.
lint:
	@./mage lint
.PHONY: lint

## Runs code-generators and unittests.
test-unit: generate
	@echo "running unit tests..."
	@./mage test:unit
.PHONY: test-unit

## Runs the Integration testsuite against the current $KUBECONFIG cluster
test-integration: export ENABLE_WEBHOOK=true
test-integration: export ENABLE_API_MOCK=true
test-integration:
	@echo "running integration tests..."
	@./mage test:deploy test:integration
.PHONY: test-integration

# legacy alias for CI/CD
test-e2e: | \
	config/deploy/deployment.yaml \
	config/deploy/api-mock/deployment.yaml \
	config/deploy/webhook/deployment.yaml \
	test-integration
.PHONY: test-e2e

## Runs the Integration testsuite against the current $KUBECONFIG cluster. Skips operator setup and teardown.
test-integration-short:
	@echo "running [short] integration tests..."
	@./mage test:integrationShort

# make sure that we install our components into the kind cluster and disregard normal $KUBECONFIG
test-integration-local: export KUBECONFIG=$(abspath $(KIND_KUBECONFIG))
## Setup a local dev environment and execute the full integration testsuite against it.
test-integration-local: | \
	dev-setup \
	prepare-addon-operator \
	prepare-addon-operator-webhook \
	prepare-api-mock \
	test-integration
.PHONY: test-integration-local

# -------------------------
##@ Development Environment
# -------------------------

## Installs all project dependencies into $(PWD)/.cache/bin
dependencies: \
	$(KIND) \
	$(CONTROLLER_GEN) \
	$(YQ) \
	$(GOIMPORTS) \
	$(GOLANGCI_LINT)
.PHONY: dependencies

## Run cmd/addon-operator-manager against $KUBECONFIG.
run-addon-operator-manager:

## Run cmd/% against $KUBECONFIG.
run-%: generate
	go run -ldflags "-w $(LD_FLAGS)" \
		./cmd/$*/main.go \
			-pprof-addr="127.0.0.1:8065" \
			-metrics-addr="0"

# make sure that we install our components into the kind cluster and disregard normal $KUBECONFIG
dev-setup: export KUBECONFIG=$(abspath $(KIND_KUBECONFIG))
## Setup a local env for feature development. (Kind, OLM, OKD Console)
dev-setup: | \
	create-kind-cluster \
	setup-olm \
	setup-okd-console
.PHONY: dev-setup

## Setup a local env for integration test development. (Kind, OLM, OKD Console, Addon Operator). Use with test-integration-short.
test-setup: | \
	dev-setup \
	setup-addon-operator
.PHONY: test-setup

## Creates an empty kind cluster to be used for local development.
create-kind-cluster: $(KIND)
	@echo "creating kind cluster addon-operator..."
	@mkdir -p .cache/integration
	@(source hack/determine-container-runtime.sh; \
		mkdir -p $(KIND_KUBECONFIG_DIR); \
		$$KIND_COMMAND create cluster \
			--kubeconfig=$(KIND_KUBECONFIG) \
			--name=$(KIND_CLUSTER_NAME); \
		echo; \
	) 2>&1 | sed 's/^/  /'
	@if [[ ! -O "$(dir KIND_KUBECONFIG)" ]]; then \
		sudo chown -R $$USER: "$(KIND_KUBECONFIG)"; \
	fi
	@if [[ ! -O "$(KIND_KUBECONFIG)" ]]; then \
		sudo chown $$USER: "$(KIND_KUBECONFIG)"; \
	fi

	@echo "post-setup for kind-cluster..."
	@(kubectl create -f config/ocp/cluster-version-operator_01_clusterversion.crd.yaml; \
		kubectl create -f config/ocp/config-operator_01_proxy.crd.yaml; \
		kubectl create -f config/ocp/cluster-version.yaml; \
		kubectl create -f config/ocp/monitoring.coreos.com_servicemonitors.yaml; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: create-kind-cluster

## Deletes the previously created kind cluster.
delete-kind-cluster: $(KIND)
	@echo "deleting kind cluster addon-operator..."
	@(source hack/determine-container-runtime.sh; \
		$$KIND_COMMAND delete cluster \
			--kubeconfig="$(KIND_KUBECONFIG)" \
			--name=$(KIND_CLUSTER_NAME); \
		rm -rf "$(KIND_KUBECONFIG)"; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: delete-kind-cluster

## Setup OLM into the currently selected cluster.
setup-olm:
	@echo "installing OLM $(OLM_VERSION)..."
	@(kubectl apply -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/$(OLM_VERSION)/crds.yaml; \
		kubectl apply -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/$(OLM_VERSION)/olm.yaml; \
		echo -e "\nwaiting for deployment/olm-operator..."; \
		kubectl wait --for=condition=available deployment/olm-operator -n olm --timeout=240s; \
		echo -e "\nwaiting for deployment/catalog-operator..."; \
		kubectl wait --for=condition=available deployment/catalog-operator -n olm --timeout=240s; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: setup-olm

## Setup the OpenShift/OKD console into the currently selected cluster.
setup-okd-console:
	@echo "installing OpenShift console :latest..."
	@(kubectl apply -f hack/openshift-console.yaml; \
		echo; \
	) 2>&1 | sed 's/^/  /'
.PHONY: setup-okd-console

## Setup Prometheus Kubernetes stack
setup-monitoring: $(HELM)
	@(kubectl create ns monitoring)
	@(helm repo add prometheus-community https://prometheus-community.github.io/helm-charts)
	@(helm repo update)
	@(helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring \
     --set grafana.enabled=false \
     --set kubeStateMetrics.enabled=false \
     --set nodeExporter.enabled=false)

## Loads the OCM API Mock into the currently selected cluster.
prepare-api-mock: \
	load-api-mock \
	config/deploy/api-mock/deployment.yaml
.PHONY: prepare-api-mock

## Loads the Addon Operator Webhook into the currently selected cluster.
prepare-addon-operator-webhook: \
	load-addon-operator-webhook \
	config/deploy/webhook/deployment.yaml
.PHONY: prepare-addon-operator-webhook

## Loads the Addon Operator into the currently selected cluster.
prepare-addon-operator: \
	load-addon-operator \
	config/deploy/deployment.yaml
.PHONY: prepare-addon-operator

## Load Addon Operator images into kind
load-addon-operator: build-image-addon-operator-manager
	@source hack/determine-container-runtime.sh; \
		$$KIND_COMMAND load image-archive \
			.cache/image/addon-operator-manager.tar \
			--name=$(KIND_CLUSTER_NAME);
.PHONY: load-addon-operator

## Load Addon Operator Webhook images into kind
load-addon-operator-webhook: build-image-addon-operator-webhook
	@source hack/determine-container-runtime.sh; \
		$$KIND_COMMAND load image-archive \
			.cache/image/addon-operator-webhook.tar \
			--name=$(KIND_CLUSTER_NAME);
.PHONY: load-addon-operator-webhook

## Load OCM API mock images into kind
load-api-mock: build-image-api-mock
	@source hack/determine-container-runtime.sh; \
		$$KIND_COMMAND load image-archive \
			.cache/image/api-mock.tar \
			--name=$(KIND_CLUSTER_NAME);
.PHONY: load-api-mock

# Template deployment for Addon Operator
config/deploy/deployment.yaml: FORCE $(YQ)
	@yq eval '.spec.template.spec.containers[0].image = "$(ADDON_OPERATOR_MANAGER_IMAGE)"' \
		config/deploy/deployment.yaml.tpl > config/deploy/deployment.yaml

# Template deployment for OCM API Mock
config/deploy/api-mock/deployment.yaml: FORCE $(YQ)
	@yq eval '.spec.template.spec.containers[0].image = "$(API_MOCK_IMAGE)"' \
		config/deploy/api-mock/deployment.yaml.tpl > config/deploy/api-mock/deployment.yaml

# Template deployment for Addon Operator Webhook
config/deploy/webhook/deployment.yaml: FORCE $(YQ)
	@yq eval '.spec.template.spec.containers[0].image = "$(ADDON_OPERATOR_WEBHOOK_IMAGE)" | .spec.template.spec.containers[0].ports[0].containerPort = $(WEBHOOK_PORT)' \
		config/deploy/webhook/deployment.yaml.tpl > config/deploy/webhook/deployment.yaml;
	@yq eval '.spec.ports[0].targetPort = $(WEBHOOK_PORT)' \
	config/deploy/webhook/service.yaml.tpl > config/deploy/webhook/service.yaml

## Loads and installs the Addon Operator into the currently selected cluster.
setup-addon-operator: prepare-addon-operator
	@echo "installing Addon Operator $(VERSION)..."
	@(source hack/determine-container-runtime.sh; \
		kubectl apply -f config/deploy; \
		echo -e "\nwaiting for deployment/addon-operator..."; \
		kubectl wait --for=condition=available deployment/addon-operator -n addon-operator --timeout=240s; \
		echo; \
	) 2>&1 | sed 's/^/  /'
ifneq ($(ENABLE_WEBHOOK), "false")
	@make prepare-addon-operator-webhook
endif
ifneq ($(ENABLE_API_MOCK), "false")
	@make prepare-api-mock
endif
ifeq ($(ENABLE_MONITORING), "true")
	@make setup-monitoring
	@(source hack/determine-container-runtime.sh; \
		kubectl apply -f config/deploy/monitoring; \
		echo; \
	) 2>&1 | sed 's/^/  /'
endif
.PHONY: setup-addon-operator

## Installs Addon Operator CRDs in to the currently selected cluster.
setup-addon-operator-crds: generate
	@for crd in $(wildcard config/deploy/*.openshift.io_*.yaml); do \
		kubectl apply -f $$crd; \
	done
.PHONY: setup-addon-operator-crds

# ---
# OLM
# ---

# Template Cluster Service Version / CSV
# By setting the container image to deploy.
config/olm/addon-operator.csv.yaml: FORCE $(YQ)
	@yq eval '.spec.install.spec.deployments[0].spec.template.spec.containers[0].image = "$(ADDON_OPERATOR_MANAGER_IMAGE)" | .spec.install.spec.deployments[1].spec.template.spec.containers[0].image = "$(ADDON_OPERATOR_WEBHOOK_IMAGE)" | .metadata.annotations.containerImage = "$(ADDON_OPERATOR_MANAGER_IMAGE)"' \
	config/olm/addon-operator.csv.tpl.yaml > config/olm/addon-operator.csv.yaml

# Bundle image contains the manifests and CSV for a single version of this operator.
# The first few lines of the CRD file need to be removed:
# https://github.com/operator-framework/operator-registry/issues/222
build-image-addon-operator-bundle: \
	clean-image-cache-addon-operator-bundle \
	config/olm/addon-operator.csv.yaml
	$(eval IMAGE_NAME := addon-operator-bundle)
	@echo "building image ${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}..."
	@(source hack/determine-container-runtime.sh; \
		mkdir -p ".cache/image/${IMAGE_NAME}/manifests"; \
		mkdir -p ".cache/image/${IMAGE_NAME}/metadata"; \
		cp -a "config/olm/addon-operator.csv.yaml" ".cache/image/${IMAGE_NAME}/manifests"; \
		cp -a "config/olm/annotations.yaml" ".cache/image/${IMAGE_NAME}/metadata"; \
		cp -a "config/docker/${IMAGE_NAME}.Dockerfile" ".cache/image/${IMAGE_NAME}/Dockerfile"; \
		tail -n"+3" "config/deploy/addons.managed.openshift.io_addons.yaml" > ".cache/image/${IMAGE_NAME}/manifests/addons.crd.yaml"; \
		tail -n"+3" "config/deploy/addons.managed.openshift.io_addonoperators.yaml" > ".cache/image/${IMAGE_NAME}/manifests/addonoperators.crd.yaml"; \
		tail -n"+3" "config/deploy/addons.managed.openshift.io_addoninstances.yaml" > ".cache/image/${IMAGE_NAME}/manifests/addoninstances.crd.yaml"; \
		$$CONTAINER_COMMAND build -t "${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}" ".cache/image/${IMAGE_NAME}"; \
		$$CONTAINER_COMMAND image save -o ".cache/image/${IMAGE_NAME}.tar" "${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}"; \
		echo) 2>&1 | sed 's/^/  /'
.PHONY: build-image-addon-operator-bundle

# Index image contains a list of bundle images for use in a CatalogSource.
# Warning!
# The bundle image needs to be pushed so the opm CLI can create the index image.
build-image-addon-operator-index: $(OPM) \
	clean-image-cache-addon-operator-index \
	| build-image-addon-operator-bundle \
	push-image-addon-operator-bundle
	$(eval IMAGE_NAME := addon-operator-index)
	@echo "building image ${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}..."
	@(source hack/determine-container-runtime.sh; \
		opm index add --container-tool $$CONTAINER_COMMAND \
		--bundles ${IMAGE_ORG}/addon-operator-bundle:${VERSION} \
		--tag ${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}; \
		$$CONTAINER_COMMAND image save -o ".cache/image/${IMAGE_NAME}.tar" "${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}"; \
		echo) 2>&1 | sed 's/^/  /'
.PHONY: build-image-addon-operator-index

# ------------------
##@ Container Images
# ------------------

## Build all images.
build-images: \
	build-image-addon-operator-manager \
	build-image-addon-operator-webhook
.PHONY: build-images

## Build and push all images.
push-images: \
	push-image-addon-operator-manager \
	push-image-addon-operator-webhook \
	push-image-addon-operator-index
.PHONY: push-images

registry-login:
ifdef JENKINS_HOME
	@(source hack/determine-container-runtime.sh; \
		echo running in Jenkins, calling $$CONTAINER_COMMAND login; \
		$$CONTAINER_COMMAND login -u="${QUAY_USER}" -p="${QUAY_TOKEN}" quay.io; \
	echo) 2>&1 | sed 's/^/  /'
endif

# App Interface specific push-images target, to run within a docker container.
app-interface-push-images:
	@echo "-------------------------------------------------"
	@echo "running in app-interface-push-images container..."
	@echo "-------------------------------------------------"
	$(eval IMAGE_NAME := app-interface-push-images)
	@(source hack/determine-container-runtime.sh; \
		$$CONTAINER_COMMAND build -t "${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}" -f "config/docker/${IMAGE_NAME}.Dockerfile" .; \
		$$CONTAINER_COMMAND run --rm \
			--privileged \
			-e JENKINS_HOME=${JENKINS_HOME} \
			-e QUAY_USER=${QUAY_USER} \
			-e QUAY_TOKEN=${QUAY_TOKEN} \
			"${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}" \
			make push-images; \
	echo) 2>&1 | sed 's/^/  /'
.PHONY: app-interface-push-images

## openshift release openshift-ci operator
openshift-ci-test-build: \
	clean-config-openshift \
	$(eval IMAGE_NAME := addon-operator-bundle)
	@echo "preparing files for config/openshift ${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}..."
	@mkdir -p "config/openshift/manifests";
	@mkdir -p "config/openshift/metadata";
	@cp "config/docker/${IMAGE_NAME}.Dockerfile" "config/openshift/${IMAGE_NAME}.Dockerfile";
	@cp "config/olm/annotations.yaml" "config/openshift/metadata";
	@cp "config/olm/addon-operator.csv.tpl.yaml" "config/openshift/manifests/addon-operator.csv.yaml";
	@tail -n"+3" "config/deploy/addons.managed.openshift.io_addons.yaml" > "config/openshift/manifests/addons.crd.yaml";
	@tail -n"+3" "config/deploy/addons.managed.openshift.io_addonoperators.yaml" > "config/openshift/manifests/addonoperators.crd.yaml";
	@tail -n"+3" "config/deploy/addons.managed.openshift.io_addoninstances.yaml" > "config/openshift/manifests/addoninstances.crd.yaml";

.SECONDEXPANSION:
# cleans the built image .tar and image build directory
clean-image-cache-%:
	@rm -rf ".cache/image/$*" ".cache/image/$*.tar"
	@mkdir -p ".cache/image/$*"

# cleans the config/openshift folder for addon-operator-bundle openshift test folder
clean-config-openshift:
	@rm -rf "config/openshift/*"

## Builds config/docker/%.Dockerfile using a binary build from cmd/%.
build-image-%: bin/linux_amd64/$$*
	@echo "building image ${IMAGE_ORG}/$*:${VERSION}..."
	@(source hack/determine-container-runtime.sh; \
		rm -rf ".cache/image/$*" ".cache/image/$*.tar"; \
		mkdir -p ".cache/image/$*"; \
		cp -a "bin/linux_amd64/$*" ".cache/image/$*"; \
		cp -a "config/docker/$*.Dockerfile" ".cache/image/$*/Dockerfile"; \
		$$CONTAINER_COMMAND build -t "${IMAGE_ORG}/$*:${VERSION}" ".cache/image/$*"; \
		$$CONTAINER_COMMAND image save -o ".cache/image/$*.tar" "${IMAGE_ORG}/$*:${VERSION}"; \
		echo; \
	) 2>&1 | sed 's/^/  /'

## Build and push config/docker/%.Dockerfile using a binary build from cmd/%.
push-image-%: registry-login build-image-$$*
	@echo "pushing image ${IMAGE_ORG}/$*:${VERSION}..."
	@(source hack/determine-container-runtime.sh; \
		$$CONTAINER_COMMAND push "${IMAGE_ORG}/$*:${VERSION}"; \
		echo pushed "${IMAGE_ORG}/$*:${VERSION}"; \
		echo; \
	) 2>&1 | sed 's/^/  /'

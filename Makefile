include boilerplate/generated-includes.mk

# Additional Deployment Image
define ADDITIONAL_IMAGE_SPECS
build/Dockerfile.webhook $(SUPPLEMENTARY_IMAGE_URI)
endef

# Operator versioning for Boilerplate
VERSION_MAJOR=1
VERSION_MINOR=15

SHELL=/bin/bash
.SHELLFLAGS=-euo pipefail -c

CONTAINER_ENGINE ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

# Dependency Versions
CONTROLLER_GEN_VERSION:=v0.6.2
OLM_VERSION:=v0.20.0
KIND_VERSION:=v0.20.0
YQ_VERSION:=v4@v4.12.0
GOIMPORTS_VERSION:=v0.12.0
GOLANGCI_LINT_VERSION:=v1.54.2
OPM_VERSION:=v1.24.0

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

PKG_BASE_IMG ?= addon-operator-package
PKG_IMG_REGISTRY ?= quay.io
PKG_IMG_ORG ?= app-sre
PKG_IMG ?= $(PKG_IMG_REGISTRY)/$(PKG_IMG_ORG)/${PKG_BASE_IMG}
PKG_IMAGETAG ?= ${SHORT_SHA}

# PATH/Bin
PROJECT_DIR:=$(shell pwd)
DEPENDENCIES:=.deps
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
ENABLE_MONITORING?="false"
ENABLE_REMOTE_STORAGE_MOCK="true"
WEBHOOK_PORT?=8080
TESTOPTS?=-cover -race -v
GOVULNCHECK_VERSION=v1.0.1

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
all:
	./mage build:all

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
clean-setup: delete-kind-cluster
	@rm -rf bin .cache
.PHONY: clean-setup

# ---------
##@ Compile
# ---------

# empty force target to ensure a target always executes.
FORCE:

# ----------------------------
# Dependencies (project local)
# ----------------------------

kind:
	./mage dependency:kind

yq:
	./mage dependency:yq

opm:
	./mage dependency:opm

helm:
	./mage dependency:helm

## Run go mod tidy in all go modules
tidy:
	@go mod tidy

# -----------
##@ Testing
# -----------

## Runs unittests.
go-test:
	@echo "running unit tests..."
	CGO_ENABLED=1 go test $(TESTOPTS) ./internal/... ./cmd/... ./pkg/... ./controllers/...
.PHONY: go-test

## Runs the Integration testsuite against the current $KUBECONFIG cluster
test-integration: export ENABLE_WEBHOOK=true
test-integration: export ENABLE_API_MOCK=true
test-integration: export EXPERIMENTAL_FEATURES=true
test-integration:
	@echo "running integration tests..."
	./mage test:integration
.PHONY: test-integration

# legacy alias for CI/CD
test-e2e:
	./mage test:integrationci
.PHONY: test-e2e

## Runs the Integration testsuite against the current $KUBECONFIG cluster. Skips operator setup and teardown.
test-integration-short:
	@echo "running [short] integration tests..."
	@go test -v -count=1 -short ./integration/...
	./mage test:integrationshort

## Setup a local dev environment and execute the full integration testsuite against it.
test-integration-local:
	./mage dev:integration
.PHONY: test-integration-local

# -------------------------
##@ Development Environment
# -------------------------

## Installs all project dependencies into $(PWD)/.deps/bin
dependencies:
	./mage dependency:all
.PHONY: dependencies

## Run cmd/addon-operator-manager against $KUBECONFIG.
run-addon-operator-manager:

## Run cmd/% against $KUBECONFIG.
run-%: generate
	go run -ldflags "-w $(LD_FLAGS)" . \
			-pprof-addr="127.0.0.1:8065" \
			-metrics-addr="0"

# make sure that we install our components into the kind cluster and disregard normal $KUBECONFIG
dev-setup: export KUBECONFIG=$(abspath $(KIND_KUBECONFIG))
## Setup a local env for feature development. (Kind, OLM, OKD Console)
dev-setup:
	./mage dev:setup
.PHONY: dev-setup

## Setup a local env for integration test development. (Kind, OLM, OKD Console, Addon Operator). Use with test-integration-short.
test-setup: | \
	dev-setup \
	setup-addon-operator
.PHONY: test-setup

## Deletes the previously created kind cluster.
delete-kind-cluster:
	./mage dev:teardown
.PHONY: delete-kind-cluster

## Setup Prometheus Kubernetes stack
setup-monitoring: helm
	@(kubectl create ns monitoring)
	@(helm repo add prometheus-community https://prometheus-community.github.io/helm-charts)
	@(helm repo update)
	-helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring --set grafana.enabled=false --set kubeStateMetrics.enabled=false --set nodeExporter.enabled=false
	@(kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.60.1/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml)
	@(helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring --set grafana.enabled=false --set kubeStateMetrics.enabled=false --set nodeExporter.enabled=false)

## Loads and installs the Addon Operator into the currently selected cluster.
setup-addon-operator:
	./mage dev:deploy
.PHONY: setup-addon-operator

## Installs Addon Operator CRDs in to the currently selected cluster.
setup-addon-operator-crds:
	@for crd in $(wildcard deploy/crds/*.openshift.io_*.yaml); do \
		kubectl apply -f $$crd; \
	done
.PHONY: setup-addon-operator-crds

# ------------------
##@ Container Images
# ------------------

## Build all images.
build-images:
	./mage build:buildimages
.PHONY: build-images

## Build and push all images.
push-images:
	./mage build:pushimages
.PHONY: push-images

# App Interface specific push-images target, to run within a docker container.
app-interface-push-images:
	@echo "-------------------------------------------------"
	@echo "running in app-interface-push-images container..."
	@echo "-------------------------------------------------"
	$(eval IMAGE_NAME := app-interface-push-images)
	@(source hack/determine-container-runtime.sh; \
		$$CONTAINER_COMMAND build -t "${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}" -f "config/docker/${IMAGE_NAME}.Dockerfile" --pull .; \
		$$CONTAINER_COMMAND run --rm \
			--privileged \
			-e JENKINS_HOME=${JENKINS_HOME} \
			-e QUAY_USER=${QUAY_USER} \
			-e QUAY_TOKEN=${QUAY_TOKEN} \
			"${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}" \
			./mage build:pushimagesonce; \
	echo) 2>&1 | sed 's/^/  /'
.PHONY: app-interface-push-images

## openshift release openshift-ci operator
openshift-ci-test-build: \
	clean-config-openshift
	@ADDON_OPERATOR_MANAGER_IMAGE=quay.io/openshift/addon-operator:latest ADDON_OPERATOR_WEBHOOK_IMAGE=quay.io/openshift/addon-operator-webhook:latest ./mage build:TemplateAddonOperatorCSV
	$(eval IMAGE_NAME := addon-operator-bundle)
	@echo "preparing files for config/openshift ${IMAGE_ORG}/${IMAGE_NAME}:${VERSION}..."
	@mkdir -p "config/openshift/manifests";
	@mkdir -p "config/openshift/metadata";
	@cp "deploy-extras/docker/${IMAGE_NAME}.Dockerfile" "config/openshift/${IMAGE_NAME}.Dockerfile";
	@cp "deploy-extras/olm/annotations.yaml" "config/openshift/metadata";
	@cp "deploy/45_metrics-service.yaml" "config/openshift/manifests/metrics.service.yaml";
	@cp "deploy/50_servicemonitor.yaml" "config/openshift/manifests/addon-operator-servicemonitor.yaml";
	@cp "deploy/35_prometheus-role.yaml" "config/openshift/manifests/prometheus-role.yaml";
	@cp "deploy/40_prometheus-rolebinding.yaml" "config/openshift/manifests/prometheus-rb.yaml";
	@cp "deploy-extras/olm/addon-operator.csv.yaml" "config/openshift/manifests/addon-operator.csv.yaml";
	@tail -n"+3" "deploy/crds/addons.managed.openshift.io_addons.yaml" > "config/openshift/manifests/addons.crd.yaml";
	@tail -n"+3" "deploy/crds/addons.managed.openshift.io_addonoperators.yaml" > "config/openshift/manifests/addonoperators.crd.yaml";
	@tail -n"+3" "deploy/crds/addons.managed.openshift.io_addoninstances.yaml" > "config/openshift/manifests/addoninstances.crd.yaml";

.SECONDEXPANSION:

## Builds config/docker/%.Dockerfile using a binary build from cmd/%.
build-image-%:
	./mage build:imagebuild $*

## Build and push config/docker/%.Dockerfile using a binary build from cmd/%.
push-image-%:
	./mage build:imagepush $*

# cleans the config/openshift folder for addon-operator-bundle openshift test folder
clean-config-openshift:
	@rm -rf "config/openshift/*"

ensure-govulncheck:
	@ls $(GOPATH)/bin/govulncheck 1>/dev/null || go install golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}

scan: ensure-govulncheck
	govulncheck ./...

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update

## Build and push only the addon-operator-package
.PHONY: build-package-push
build-package-push: 
	hack/build-package.sh ${PKG_IMG}:${PKG_IMAGETAG}

.PHONY: build-package
build-package:
		$(CONTAINER_ENGINE) build -t $(PKG_IMG):$(PKG_IMAGETAG) -f $(join $(CURDIR),/hack/hypershift/package/addon-operator-package.Containerfile) . && \
		$(CONTAINER_ENGINE) tag $(PKG_IMG):$(PKG_IMAGETAG) $(PKG_IMG):latest

.PHONY: skopeo-push
skopeo-push-package:
	@if [[ -z $$QUAY_USER || -z $$QUAY_TOKEN ]]; then \
		echo "You must set QUAY_USER and QUAY_TOKEN environment variables" ;\
		echo "ex: make QUAY_USER=value QUAY_TOKEN=value $@" ;\
		exit 1 ;\
	fi
	# QUAY_USER and QUAY_TOKEN are supplied as env vars
	skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
		"docker-daemon:${PKG_IMG}:${PKG_IMAGETAG}" \
		"docker://${PKG_IMG}:latest"
	skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
		"docker-daemon:${PKG_IMG}:${PKG_IMAGETAG}" \
		"docker://${PKG_IMG}:${PKG_IMAGETAG}"

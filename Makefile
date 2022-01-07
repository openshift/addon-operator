SHELL=/bin/bash
.SHELLFLAGS=-euo pipefail -c

# Build Flags
export CGO_ENABLED:=0
BRANCH=$(shell git rev-parse --abbrev-ref HEAD)
SHORT_SHA=$(shell git rev-parse --short HEAD)
VERSION?=${SHORT_SHA}
BUILD_DATE=$(shell date +%s)
MODULE:=github.com/openshift/addon-operator
GOFLAGS=

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


# ---------
##@ General
# ---------

# Default build target - must be first!
# Executed by Prow to build binaries before the container images are build.
all:
	@./mage build:all

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
test-e2e:
	test-integration
.PHONY: test-e2e

## Runs the Integration testsuite against the current $KUBECONFIG cluster. Skips operator setup and teardown.
test-integration-short:
	@echo "running [short] integration tests..."
	@./mage test:integrationShort

## Setup a local dev environment and execute the full integration testsuite against it.
test-integration-local:
	@./mage dev:integrationTests
.PHONY: test-integration-local

# -------------------------
##@ Development Environment
# -------------------------

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
dev-setup:
	@./mage dev:empty
.PHONY: dev-setup

## Setup a local env for integration test development. (Kind, OLM, OKD Console, Addon Operator). Use with test-integration-short.
test-setup:
	@./mage dev:testing
.PHONY: test-setup

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

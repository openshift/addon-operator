# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building and Testing
- `make all` - Default build target that builds all components
- `./mage build:all` - Alternative build command via mage
- `make go-test` - Run unit tests for internal/, cmd/, pkg/, and controllers/ packages
- `make test-integration` - Run integration tests against current $KUBECONFIG cluster
- `make test-integration-short` - Run integration tests without operator setup/teardown
- `make test-integration-local` - Setup local dev environment and run full integration suite
- `make tidy` - Run go mod tidy (used by pre-commit hooks)

### Development Environment
- `make dev-setup` - Setup kind cluster with OLM and OpenShift Console for out-of-cluster development
- `make test-setup` - Complete setup: kind cluster + OLM + Console + compile + build containers + install operator
- `make run-addon-operator-manager` - Run operator out-of-cluster (requires dev-setup first)
- `make setup-addon-operator-crds` - Install CRDs into cluster
- `export KUBECONFIG=$PWD/.cache/dev-env/kubeconfig.yaml` - Use local kind cluster

### Code Quality
- `make container-generate` - Generate CRDs via containerized controller-gen
- `make container-lint` - Run linters via containerized golangci-lint
- `make generate-bundle` - Generate OLM bundle after modifying operator manifests

### Environment Variables for Testing
- `ENABLE_WEBHOOK=true/false` - Deploy webhook server with admission webhooks
- `WEBHOOK_PORT=<PORT>` - Port for webhook server
- `ENABLE_API_MOCK=true/false` - Deploy mock OCM API server for UpgradePolicy testing
- `ENABLE_MONITORING=true/false` - Deploy kube-prometheus monitoring stack

## Architecture Overview

### Core Components
- **Addon Operator**: Kubernetes controller that orchestrates addon lifecycles on managed OpenShift clusters
- **Custom Resources**: Three main CRDs - Addon, AddonInstance, and AddonOperator
- **Controllers**: Separate controller packages for each CRD in `controllers/` directory
- **Webhooks**: Admission webhooks in `internal/webhooks/` for validation and mutation

### Project Structure
- `api/v1alpha1/` - Kubernetes API definitions and CRD types
- `controllers/` - Controller implementations for each custom resource
- `internal/` - Internal packages including metrics, OCM client, feature toggles, webhooks
- `cmd/` - Entry points for different operator components
- `deploy/` - Production-grade Kubernetes manifests
- `integration/` - Integration test suites
- `magefiles/` - Mage build automation scripts

### Key Internal Packages
- `internal/ocm/` - OCM (OpenShift Cluster Manager) API client integration
- `internal/metrics/` - Prometheus metrics collection and reporting
- `internal/featuretoggle/` - Feature flag management system
- `internal/webhooks/` - Kubernetes admission webhook implementations

### Dependencies and Tooling
- Uses Boilerplate project for standardized tooling and conventions
- Built with controller-runtime framework
- Integrates with OLM (Operator Lifecycle Manager)
- Supports both podman and docker container runtimes
- Pre-commit hooks enforce code quality (go-fmt, go-mod-tidy)

### Testing Strategy
- Unit tests cover internal logic and controller business logic
- Integration tests run against real Kubernetes clusters via kind
- Supports both in-cluster and out-of-cluster development workflows
- Mock OCM API server available for testing upgrade policies
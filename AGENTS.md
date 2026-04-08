# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Addon Operator is an OSD (OpenShift Dedicated) operator that orchestrates the lifecycle of Addons in managed OpenShift clusters. It manages addon installation, configuration, monitoring, and health reporting. The operator communicates addon status to OpenShift Cluster Manager (OCM) and exposes Prometheus metrics.

**API Group**: `addons.managed.openshift.io/v1alpha1`

## Build & Development Commands

This project uses **Mage** (Go-based task runner) wrapped by Make. It subscribes to [openshift/boilerplate](https://github.com/openshift/boilerplate) conventions via `boilerplate/generated-includes.mk`.

### Building

```bash
make all                    # Build everything (via ./mage build:all)
make build-images           # Build container images
```

### Testing

```bash
# Unit tests (requires CGO_ENABLED=1 for race detector)
make go-test
# Equivalent: CGO_ENABLED=1 go test -cover -v -race ./internal/... ./cmd/... ./pkg/... ./controllers/...

# Run a single unit test
CGO_ENABLED=1 go test -v -run TestMyFunction ./controllers/addon/...

# Integration tests (requires Kind cluster with operator deployed)
make test-integration                # Full integration suite
make test-integration-short          # Skip operator setup/teardown (use after make test-setup)
./mage test:integrationrun "TestIntegration/TestAddon"  # Run specific integration test
make test-integration-local          # Setup cluster + deploy operator + run tests (all-in-one)
```

### Linting & Code Generation

```bash
# Linting (use container versions for CI-consistent results)
make container-lint         # golangci-lint via boilerplate's pinned version
./mage test:lint            # Local lint

# Code generation (CRDs, deepcopy, OpenAPI)
make container-generate     # Via boilerplate's pinned controller-gen
make generate-check         # Verify generated code is up to date

# OLM bundle generation
make generate-bundle
```

### Local Development (out-of-cluster)

```bash
make dev-setup                                          # Create Kind cluster + install OLM
make setup-addon-operator-crds                          # Install CRDs into cluster
export KUBECONFIG=$PWD/.cache/dev-env/kubeconfig.yaml
export ADDON_OPERATOR_NAMESPACE=openshift-addon-operator
make run-addon-operator-manager                         # Run operator locally
```

### Integration Test Development Loop

```bash
make test-setup              # Kind + OLM + build + deploy operator
# Edit tests in integration/
make test-integration-short  # Re-run without re-deploying operator
```

### Flags for Local Deployment

- `ENABLE_WEBHOOK=true/false` — Deploy webhook server
- `ENABLE_API_MOCK=true/false` — Deploy mock OCM API server
- `ENABLE_MONITORING=true/false` — Deploy kube-prometheus monitoring stack
- `CONTAINER_RUNTIME=podman|docker` — Override auto-detected container runtime

## Architecture

### CRDs

- **Addon** — Primary resource. Defines addon display name, version, namespaces, install spec (OLM or PackageOperator), monitoring config, upgrade policy, and secret propagation.
- **AddonOperator** — Cluster-scoped singleton (`addon-operator`). Global configuration including feature flags and pause state.
- **AddonInstance** — Per-addon health/heartbeat tracking. Created by the operator in addon namespaces.

### Controllers

Three controllers bootstrapped in `main.go` → `initReconcilers()`:

1. **AddonReconciler** (`controllers/addon/`) — Main reconciliation logic. Uses an ordered **sub-reconciler chain** where each sub-reconciler handles one concern:
   - `addonDeletionReconciler` — Multi-handler deletion (legacy + addon-instance acknowledgment)
   - `namespaceReconciler` — Create/adopt namespaces
   - `addonSecretPropagationReconciler` — Copy secrets from operator namespace to addon namespaces
   - `addonInstanceReconciler` — Create/manage AddonInstance objects
   - `olmReconciler` — Manage CatalogSource, OperatorGroup, Subscription
   - `monitoringFederationReconciler` — Federate addon Prometheus metrics

   Sub-reconcilers implement the `addonReconciler` interface and execute serially by `Order()`. Each returns a `subReconcilerResult` (nil/retry/stop/requeueAfter).

2. **AddonOperatorReconciler** (`controllers/addonoperator/`) — Manages global pause, OCM client lifecycle, feature toggles.

3. **AddonInstanceController** (`controllers/addoninstance/`) — Phase-based reconciliation using serial phases (currently: `PhaseCheckHeartbeat`).

### OLM Phase Files

The `controllers/addon/phase_ensure_*.go` files handle specific OLM resource reconciliation (CatalogSource, OperatorGroup, Subscription, NetworkPolicy).

### Feature Toggle System

`internal/featuretoggle/` provides a hook system where features can modify the scheme and reconciler options at startup. Feature toggles are read from the `AddonOperator` CR's `.spec.featureFlags` field. Each toggle has `PreManagerSetupHandle` and `PostManagerSetupHandle` hooks.

### OCM Integration

`internal/ocm/` contains the OCM API client. The operator reports addon status, processes upgrade policies, and syncs addon health to OCM. Status reporting is controlled by `ENABLE_STATUS_REPORTING` and `ENABLE_UPGRADEPOLICY_STATUS` environment variables.

### Operator Bootstrap (`main.go`)

- Reads `AddonOperator` CR to initialize feature toggles before creating the manager
- Manager binds: metrics on `:8080`, health probes on `:8081`, webhooks on `:9443`, pprof on `127.0.0.1:8070`
- Leader election via leases (`8a4hp84a6s.addon-operator-lock`)
- Secrets cache is filtered by label `controllers.CommonCacheLabel` to avoid caching all cluster secrets
- Namespace is sourced from: `--namespace` flag → `ADDON_OPERATOR_NAMESPACE` env → in-cluster service account namespace

## Key Conventions

- **FIPS_ENABLED=true** — Build uses FIPS-compliant configuration
- **Boilerplate subscriber** — Standardized CI targets (`container-generate`, `container-lint`, `container-test`) are inherited from boilerplate. Run `make boilerplate-update` to pull latest conventions.
- **Test coverage minimum**: 70% (enforced per MAINTAINING.md)
- **Pre-commit hooks** configured in `.pre-commit-config.yaml` (go-fmt, go-mod-tidy, large file checks)
- **golangci-lint** config in `.golangci.yaml` enables `nilnil`, presets `bugs` and `unused`
- **Container runtime**: Auto-detects `podman` then `docker`. Override with `CONTAINER_RUNTIME`.
- **Versioning**: `VERSION_MAJOR=1`, `VERSION_MINOR=15`. Binary version injected via LD_FLAGS from git metadata.

## Testing Patterns

- Unit tests use `testify` assertions alongside standard Go testing
- Integration tests live in `integration/` and use `github.com/mt-sre/devkube` for Kind cluster management
- Mock OCM API server: `cmd/api-mock/`
- Mock Prometheus remote storage: `cmd/prometheus-remote-storage-mock/`
- Test fixtures: `integration/fixtures_test.go` and `deploy-extras/` for example CRs

## CI/CD

- **Tekton pipelines** (`.tekton/`) for PR and push builds via Konflux
- **Prow** integration via OWNERS file for review/approval
- **Continuous deployment** via Red Hat AppSRE's app-interface (not in this repo)
- Images pushed to `quay.io/app-sre/addon-operator-manager` and `quay.io/app-sre/addon-operator-webhook`

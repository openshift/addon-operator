<p align="center">
	<img src="docs/logo/addon-operator-github.png" width=400px>
</p>

<p align="center">
  	<img src="https://goreportcard.com/badge/github.com/openshift/addon-operator"/>
	<img src="https://prow.ci.openshift.org/badge.svg?jobs=pull-ci-openshift-addon-operator-main*"/>
	<img src="https://codecov.io/gh/openshift/addon-operator/branch/main/graph/badge.svg"/>
 	<img src="https://img.shields.io/github/license/openshift/addon-operator"/>
</p>

---

Addon Operator coordinates the lifecycle of Addons in managed OpenShift.

## Index
- [Overview](#overview)
- [Operator Manifests](#operator-manifests)
- [Development](#development)
	- [Prerequisites](#prerequisites-and-dependencies)
	- [Commiting](#committing)
	- [Container Make](#container-make)
	- [Quickstart](#quickstart--develop-integration-tests)
	- [Iterate fast](#iterate-fast)
	- [OLM Bundle and Operator CSV](#olm-bundle-and-operator-csv)
	- [Unit testing](#unit-testing)
- [Monitoring](#monitoring-and-metrics)
- [Deployment](r#deployment)
- [Troubleshooting](#troubleshooting)
- [Additional References](#additional-references)

## Overview

Addon Operator is an OSD operator that orchestrates the lifecycle of Addons and is an integral part of the Addons Flow architecture.

Addon Operator is a subscriber of [Boilerplate](https://github.com/openshift/boilerplate) and derives most of the standardized artifacts from Boilerplate's [openshift/golang-osd-operator](https://github.com/openshift/boilerplate/blob/master/boilerplate/openshift/golang-osd-operator) convention.

## Operator Manifests

All operator related manifests reside in the `deploy` folder of the repository. These are all production-grade manifests.

## Development

All development tooling can be accessed via `make`, use `make help` to get an overview of all supported targets.

### Prerequisites and Dependencies

To contribute new features or add/run tests, `podman` or `docker` and the `go` tool chain need to be present on the system.

Dependencies are loaded as required and are kept local to the project in the `.cache` directory and you can setup or update all dependencies via `make dependencies`

A few dependencies, namely `controller-gen`, `golangci-lint` etc. with standardized versions are inherited from the Boilerplate convention.

Updating dependency versions at the top of the `Makefile` will automatically update the dependencies when they are required.

If both `docker` and `podman` are present you can explicitly force the container runtime by setting `CONTAINER_RUNTIME`.

e.g.:
```.mk
CONTAINER_RUNTIME=docker make dev-setup
```

### Committing

Before making your first commit, please consider installing [pre-commit](https://pre-commit.com/) and run `pre-commit install` in the project directory.

Pre-commit is running some basic checks for every commit and makes our code reviews easier and prevents wasting CI resources.

### Container Make

While you can run make targets locally during development to test your code changes. However, differences in platforms and environments may lead to unpredictable results. Therefore Boilerplate provides a utility to run targets in a container environment that is designed to be as similar as possible to CI.

- To generate the CRDs via Boilerplate's standardized `controller-gen` version:

```shell
make container-generate
```

- To run linters via Boilerplate's standardized `golangci-lint` version:

```shell
make container-lint
```

### Quickstart / Develop Integration tests

Just wanting to play with the operator deployed on a cluster?

```shell
# In checkout directory:
make test-setup
```

This command will:

1. Setup a cluster via kind
2. Install OLM and OpenShift Console
3. Compile your checkout
4. Build containers
5. Load them into the kind cluster (no registry needed)
6. Install the Addon Operator

This will give you a quick environment for playing with the operator.

You can also use it to develop integration tests, against a complete setup of the Addon Operator:

```.mk
# edit tests

# Run all integration tests and skip setup and teardown,
# as the operator is already installed by: make test-setup
make test-integration-short

# repeat!
```

If you want to setup the cluster, install the operator and run integration tests all in one go, then use the target:

```.mk
make test-integration-local
```

The `make` tooling offers the following flags to tweak your local in-cluster installation of the AddonOperator:

- `ENABLE_WEBHOOK=true/false` - Deploy the AddonOperator webhook server that runs Admission webhooks.
- `WEBHOOK_PORT=<PORT>` - Port to use while running the webhook server
- `ENABLE_API_MOCK=true/false` - Deploy the mock OCM API server for testing and validating the UpgradePolicy flow
- `ENABLE_MONITORING=true/false` - Deploy the kube-prometheus monitoring stack for adding / testing AddonOperator metrics

### Iterate fast

To iterate fast on code changes and experiment, the operator can also run out-of-cluster. This way we don't have to rebuild images, load them into the cluster and redeploy the operator for every code change.

Prepare the environment:

```shell
make dev-setup
```

This command will:

1. Setup a cluster via kind
2. Install OLM and OpenShift Console

```shell
# Install Addon Operator CRDs
# into the cluster.
make setup-addon-operator-crds

# Make sure we run against the new kind cluster.
export KUBECONFIG=$PWD/.cache/dev-env/kubeconfig.yaml

# Set Addon operator namespace environment variable
export ADDON_OPERATOR_NAMESPACE=openshift-addon-operator

# Run the operator out-of-cluster:
# Mind your `KUBECONFIG` environment variable!
make run-addon-operator-manager
```
### OLM Bundle and Operator CSV

The `bundle` folder reflects the Addon Operator's OLM bundle.

To generate the OLM bundle, run:

```shell
make generate-bundle
```
As a developer, each time you modify the operator manifest(s) that reside in the `deploy` folder make sure you generate a new bundle to generate the CSV with the manifest updates.

**Note:** This bundle is only intended for use with Prow CI to install the operator and run the end-to-end tests against a commit. This is not deployed to production. For production, we leverage bundle manifests that reside in app-interface that is dynamically generated in the continuous integration pipeline.

### Unit testing

We can run all the unit tests and mock tests locally. This way we can test the testable parts of the operator, individually and independently for proper operation.

```shell
# To run all the unit tests and mock tests
make test-unit
```
**Warning:**
- Your code runs as `cluster-admin`, you might run into permission errors when running in-cluster.
- Code-Generators need to be re-run and CRDs re-applied via `make setup-addon-operator-crds` when code under `./api` is changed.

## Monitoring and metrics

The Addon Operator is instrumented with the prometheus-client provided by controller-runtime to record some useful Addon metrics.

| Metric name                                 | Type       | Description                                                                             |
|---------------------------------------------|------------|-----------------------------------------------------------------------------------------|
| `addon_operator_addons_count`               | `GaugeVec` | Total number of Addon installations, grouped by 'available', 'paused' and 'total'       |
| `addon_operator_paused`                     | `Gauge`    | A boolean that tells if the AddonOperator is paused (1 - paused; 0 - unpaused)          |
| `addon_operator_ocm_api_requests_durations` | `Summary`  | OCM API request latencies in microseconds. Grouped using tail-latencies (p50, p90, p99) |
| `addon_operator_addon_health_info`          | `GaugeVec` | Addon Health information (0 - Unhealthy; 1 - Healthy; 2 - Unknown)                      |

See [Quickstart](#quickstart--develop-integration-tests) for instructions on how to setup a local monitoring stack for development / testing.

## How to: Test metrics locally

The Addon Operator metrics can be exposed locally by port forwarding the `addon-operator-metrics` service which allows users to access the service running inside a Kubernetes cluster from their local machine.

- Run Addon Operator locally:

```bash
export ENABLE_MONITORING=true

make test-setup
```

- Ensure you have the `KUBECONFIG` environment variable setup:

```bash
export KUBECONFIG=/.cache/dev-env/kubeconfig.yaml
```

- Forward traffic from port `8443` on the `addon-operator-metrics` service:

```bash
kubectl port-forward service/addon-operator-metrics -n addon-operator 8443:8443
```

- Fetch the metrics:

```bash
curl -k http://localhost:8443/metrics
```

## Deployment

Addon Operator releasing/deployment are fully automated in integration and staging environments.

Once a pull request is merged, the operator & OLM registry images are built and pushed via the CI followed by which a new OLM bundle is generated.

A new version of the operator is progressively deployed into integration & staging on every pull request merge.

Addon Operator continuous deployment is orchestrated via Red Hat AppSRE's proprietary component app-interface.

## Troubleshooting

**[Set `nf_conntrack_max`](https://github.com/kubernetes-sigs/kind/issues/2240)**

When using docker to spin a new Kind cluster, `kube-proxy` would not start throwing this error:

```
I0511 11:47:28.965997       1 conntrack.go:100] Set sysctl 'net/netfilter/nf_conntrack_max' to XXXXXX
F0511 11:47:28.966114       1 server.go:495] open /proc/sys/net/netfilter/nf_conntrack_max: permission denied
```

Make sure to:
`sudo sysctl net/netfilter/nf_conntrack_max=<value>`, and add a drop-in file to `/etc/sysctl.d/99-custom.conf` to set the kernel parameters permanently.

## Additional References

- [API reference](https://github.com/openshift/addon-operator/blob/main/docs/api-reference/_index.md)

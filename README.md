# Addon Operator

<p align="center">
	<img src="docs/logo/addon-operator-github.png" width=400px>
</p>

<p align="center">
	<img src="https://prow.ci.openshift.org/badge.svg?jobs=pull-ci-openshift-addon-operator-main*">
	<img src="https://img.shields.io/github/license/openshift/addon-operator"/>
	<img src="https://img.shields.io/badge/Coolness%20Factor-Over%209000!-blue"/>
</p>

---

Addon Operator coordinates the lifecycle of Addons in managed OpenShift.

---

## Index

- [API reference](https://github.com/openshift/addon-operator/blob/main/docs/api-reference/_index.md)
- [Development](https://github.com/openshift/addon-operator#development)
	- [Prerequisites](https://github.com/openshift/addon-operator#prerequisites-and-dependencies)
	- [Commiting](https://github.com/openshift/addon-operator#committing)
	- [Quickstart](https://github.com/openshift/addon-operator#quickstart--develop-integration-tests)
	- [Iterate fast](https://github.com/openshift/addon-operator#iterate-fast)
	- [Unit test](https://github.com/openshift/addon-operator#unit-test)
- [Troubleshooting](https://github.com/openshift/addon-operator#troubleshooting)
- [Monitoring](https://github.com/openshift/addon-operator#monitoring-and-metrics)
- [Releasing](https://github.com/openshift/addon-operator#releasing)
- [Deployment](https://github.com/openshift/addon-operator#deployment)

## Development

All development tooling can be accessed via `make`, use `make help` to get an overview of all supported targets.

This development tooling is currently used on Linux amd64, please get in touch if you need help developing from another Operating system or architecture.

### Prerequisites and Dependencies

To contribute new features or add/run tests, `podman` or `docker` and the `go` tool chain need to be present on the system.

Dependencies are loaded as required and are kept local to the project in the `.cache` directory and you can setup or update all dependencies via `make dependencies`

Updating dependency versions at the top of the `Makefile` will automatically update the dependencies when they are required.

If both `docker` and `podman` are present you can explicitly force the container runtime by setting `CONTAINER_RUNTIME`.

e.g.:
```
CONTAINER_RUNTIME=docker make dev-setup
```

### Committing

Before making your first commit, please consider installing [pre-commit](https://pre-commit.com/) and run `pre-commit install` in the project directory.

Pre-commit is running some basic checks for every commit and makes our code reviews easier and prevents wasting CI resources.

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

```shell
# edit tests

# Run all integration tests and skip setup and teardown,
# as the operator is already installed by: make test-setup
make test-integration-short

# repeat!
```

The `make` tooling offers the following flags to tweak your local in-cluster installation of the AddonOperator:

- `ENABLE_WEBHOOK=true/false`: Deploy the AddonOperator webhook server that runs Admission webhooks
- `WEBHOOK_PORT=<PORT>`: Port to use while running the webhook server
- `ENABLE_API_MOCK=true/false`: Deploy the mock OCM API server for testing and validating the UpgradePolicy flow
- `ENABLE_MONITORING=true/false`: Deploy the kube-prometheus monitoring stack for adding / testing AddonOperator metrics

### Iterate fast!

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
export KUBECONFIG=$PWD/.cache/dev-env/kubeconfig

# Set Addon operator namespace environment variable
export ADDON_OPERATOR_NAMESPACE=addon-operator

# Run the operator out-of-cluster:
# Mind your `KUBECONFIG` environment variable!
make run-addon-operator-manager
```

### Unit test

To run all the unit test and mock test into the local system. This way we can test the testable parts of the operator, individually and independently for proper operation.

```shell
# To run all the unit tests and mock tests
make test-unit
```
**Warning:**
- Your code runs as `cluster-admin`, you might run into permission errors when running in-cluster.
- Code-Generators need to be re-run and CRDs re-applied via `make setup-addon-operator-crds` when code under `./apis` is changed.

## Troubleshooting

**[Set `nf_conntrack_max`](https://github.com/kubernetes-sigs/kind/issues/2240)**

When using docker to spin a new Kind cluster, `kube-proxy` would not start throwing this error:

```
I0511 11:47:28.965997       1 conntrack.go:100] Set sysctl 'net/netfilter/nf_conntrack_max' to XXXXXX
F0511 11:47:28.966114       1 server.go:495] open /proc/sys/net/netfilter/nf_conntrack_max: permission denied
```

Make sure to:
`sudo sysctl net/netfilter/nf_conntrack_max=<value>`, and add a drop-in file to `/etc/sysctl.d/99-custom.conf` to set the kernel parameters permanently.

## Monitoring and metrics

The Addon Operator is instrumented with the prometheus-client provided by controller-runtime to record some useful Addon metrics.

| Metric name                                 | Type       | Description                                                                             |
|---------------------------------------------|------------|-----------------------------------------------------------------------------------------|
| `addon_operator_addons_count`               | `GaugeVec` | Total number of Addon installations, grouped by 'available', 'paused' and 'total'       |
| `addon_operator_paused`                     | `Gauge`    | A boolean that tells if the AddonOperator is paused (1 - paused; 0 - unpaused)          |
| `addon_operator_ocm_api_requests_durations` | `Summary`  | OCM API request latencies in microseconds. Grouped using tail-latencies (p50, p90, p99) |
| `addon_operator_addon_health_info`          | `GaugeVec` | Addon Health information (0 - Unhealthy; 1 - Healthy; 2 - Unknown)                      |

See [Quickstart](https://github.com/openshift/addon-operator#quickstart--develop-integration-tests) for instructions on how to setup a local monitoring stack for development / testing.

## How to: Test metrics locally

The Addon Operator metrics can be exposed locally by port forwarding the `addon-operator-metrics` service which allows users to access the service running inside a Kubernetes cluster from their local machine.

- Run Addon Operator locally:

```bash
➜ export ENABLE_MONITORING=true

➜ make test-setup
```

- Ensure you have the `KUBECONFIG` environment variable setup:

```bash
➜ export KUBECONFIG=/.cache/dev-env/kubeconfig.yaml
```

- Forward traffic from port `8443` on the `addon-operator-metrics` service:

```bash
➜ kubectl port-forward service/addon-operator-metrics -n addon-operator 8443:8443
```

- Fetch the metrics:

```bash
➜ curl -k https://localhost:8443/metrics
```

## Releasing

```sh
# 1. edit VERSION file
vim VERSION

# 2. update packaging files and prepare release
./mage prepare_release

# 3. create a new PR in openshift/addon-operator

# 4. create a Github Release/Git Tag on the latest commit of the main branch
```

## Deployment

Addon Operator is deployed via app-interface.
Example MR updating staging: https://gitlab.cee.redhat.com/service/app-interface/-/merge_requests/36630

Example MR updating production: https://gitlab.cee.redhat.com/service/app-interface/-/merge_requests/36743

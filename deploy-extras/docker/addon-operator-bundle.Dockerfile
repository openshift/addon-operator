FROM registry.access.redhat.com/ubi8/ubi:8.8-1032

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=addon-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.29.0
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v3

# Copy files to locations specified by labels.
ADD deploy/crds/*.yaml /manifests/
ADD deploy/50_servicemonitor.yaml /manifests/
ADD deploy/45_metrics-service.yaml /manifests/
ADD deploy/35_prometheus-role.yaml /manifests/
ADD deploy/40_prometheus-rolebinding.yaml /manifests/
ADD deploy-extras/olm/addon-operator.csv.yaml /manifests/
ADD deploy-extras/olm/annotations.yaml /metadata/annotations.yaml

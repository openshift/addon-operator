apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: observability-operator-catalog
  namespace: openshift-observability-operator
spec:
  displayName: Red Hat Observability Operator
  image: quay.io/rhobs/observability-operator-catalog:${VERSION}
  publisher: OSD Red Hat Addons
  sourceType: grpc

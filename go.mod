module github.com/openshift/addon-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.1.2
	github.com/operator-framework/api v0.8.1
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.26.0
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.22.3
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.3
	k8s.io/client-go v0.22.3
	k8s.io/kubectl v0.22.3
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	sigs.k8s.io/controller-runtime v0.10.2
	sigs.k8s.io/yaml v1.2.0
)

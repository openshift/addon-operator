module github.com/openshift/addon-operator

go 1.16

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/go-logr/logr v1.2.2
	github.com/go-logr/stdr v1.2.2
	github.com/gorilla/mux v1.8.0
	github.com/magefile/mage v1.12.1
	github.com/mt-sre/devkube v0.3.0
	github.com/openshift/addon-operator/apis v0.0.0-20220111092509-93ca25c9359f
	github.com/openshift/api v0.0.0-20211122204231-b094ceff1955
	github.com/operator-framework/api v0.8.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.56.2
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/rhobs/monitoring-stack-operator v0.0.8-0.20220517000611-e9e78612d82f
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.23.5
	k8s.io/apiextensions-apiserver v0.23.0
	k8s.io/apimachinery v0.23.5
	k8s.io/client-go v0.23.3
	k8s.io/kubectl v0.23.3
	k8s.io/utils v0.0.0-20211116205334-6203023598ed
	sigs.k8s.io/controller-runtime v0.11.0
	sigs.k8s.io/yaml v1.3.0
)

replace github.com/openshift/addon-operator/apis => ./apis

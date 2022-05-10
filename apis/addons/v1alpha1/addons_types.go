package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddonSpec defines the desired state of Addon.
type AddonSpec struct {
	// Human readable name for this addon.
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// Version of the Addon to deploy.
	// Used for reporting via status and metrics.
	// +optional
	Version string `json:"version,omitempty"`

	// Pause reconciliation of Addon when set to True
	// +optional
	Paused bool `json:"pause"`

	// Defines a list of Kubernetes Namespaces that belong to this Addon.
	// Namespaces listed here will be created prior to installation of the Addon and
	// will be removed from the cluster when the Addon is deleted.
	// Collisions with existing Namespaces are NOT allowed.
	Namespaces []AddonNamespace `json:"namespaces,omitempty"`

	// Defines how an Addon is installed.
	// This field is immutable.
	Install AddonInstallSpec `json:"install"`

	// ResourceAdoptionStrategy coordinates resource adoption for an Addon
	// Originally introduced for coordinating fleetwide migration on OSD with pre-existing OLM objects.
	// NOTE: This field is for internal usage only and not to be modified by the user.
	// +kubebuilder:validation:Enum={"Prevent","AdoptAll"}
	ResourceAdoptionStrategy ResourceAdoptionStrategyType `json:"resourceAdoptionStrategy,omitempty"`

	// UpgradePolicy enables status reporting via upgrade policies.
	UpgradePolicy *AddonUpgradePolicy `json:"upgradePolicy,omitempty"`

	// Defines how an addon is monitored.
	Monitoring *MonitoringSpec `json:"monitoring,omitempty"`

	// Settings for propagating secrets from the Addon Operator install namespace into Addon namespaces.
	SecretPropagation *AddonSecretPropagation `json:"secretPropagation,omitempty"`
}

type AddonSecretPropagation struct {
	Secrets []AddonSecretPropagationReference `json:"secrets"`
}

type AddonSecretPropagationReference struct {
	// Source secret name in the Addon Operator install namespace.
	SourceSecret corev1.LocalObjectReference `json:"sourceSecret"`
	// Destination secret name in every Addon namespace.
	DestinationSecret corev1.LocalObjectReference `json:"destinationSecret"`
}

type AddonUpgradePolicy struct {
	// Upgrade policy id.
	ID string `json:"id"`
}

type AddonUpgradePolicyValue string

const (
	AddonUpgradePolicyValueStarted   AddonUpgradePolicyValue = "started"
	AddonUpgradePolicyValueCompleted AddonUpgradePolicyValue = "completed"
)

// Tracks the last state last reported to the Upgrade Policy endpoint.
type AddonUpgradePolicyStatus struct {
	// Upgrade policy id.
	ID string `json:"id"`
	// Upgrade policy value.
	Value AddonUpgradePolicyValue `json:"value"`
	// The most recent generation a status update was based on.
	ObservedGeneration int64 `json:"observedGeneration"`
}

type MonitoringSpec struct {
	// Configuration parameters to be injected in the ServiceMonitor used for federation.
	// The target prometheus server found by matchLabels needs to serve service-ca signed TLS traffic
	// (https://docs.openshift.com/container-platform/4.6/security/certificate_types_descriptions/service-ca-certificates.html),
	// and it needs to be runing inside the namespace specified by `.monitoring.federation.namespace`
	// with the service name 'prometheus'.
	Federation *MonitoringFederationSpec `json:"federation,omitempty"`
}

type MonitoringFederationSpec struct {
	// Namespace where the prometheus server is running.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// The name of the service port fronting the prometheus server.
	// +kubebuilder:validation:MinLength=1
	PortName string `json:"portName"`

	// List of series names to federate from the prometheus server.
	// +listType:set
	MatchNames []string `json:"matchNames"`

	// List of labels used to discover the prometheus server(s) to be federated.
	// +kubebuilder:validation:MinProperties=1
	MatchLabels map[string]string `json:"matchLabels"`
}

type ResourceAdoptionStrategyType string

// known resource adoption strategy types
const (
	ResourceAdoptionPrevent  ResourceAdoptionStrategyType = "Prevent"
	ResourceAdoptionAdoptAll ResourceAdoptionStrategyType = "AdoptAll"
)

// AddonInstallSpec defines the desired Addon installation type.
type AddonInstallSpec struct {
	// Type of installation.
	// +kubebuilder:validation:Enum={"OLMOwnNamespace","OLMAllNamespaces"}
	Type AddonInstallType `json:"type"`
	// OLMAllNamespaces config parameters. Present only if Type = OLMAllNamespaces.
	OLMAllNamespaces *AddonInstallOLMAllNamespaces `json:"olmAllNamespaces,omitempty"`
	// OLMOwnNamespace config parameters. Present only if Type = OLMOwnNamespace.
	OLMOwnNamespace *AddonInstallOLMOwnNamespace `json:"olmOwnNamespace,omitempty"`
}

// Common Addon installation parameters.
type AddonInstallOLMCommon struct {
	// Namespace to install the Addon into.
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Defines the CatalogSource image.
	// +kubebuilder:validation:MinLength=1
	CatalogSourceImage string `json:"catalogSourceImage"`

	// Channel for the Subscription object.
	// +kubebuilder:validation:MinLength=1
	Channel string `json:"channel"`

	// Name of the package to install via OLM.
	// OLM will resove this package name to install the matching bundle.
	// +kubebuilder:validation:MinLength=1
	PackageName string `json:"packageName"`

	// Reference to a secret of type kubernetes.io/dockercfg or kubernetes.io/dockerconfigjson
	// in the addon operators installation namespace.
	// The secret referenced here, will be made available to the addon in the addon installation namespace,
	// as addon-pullsecret prior to installing the addon itself.
	PullSecretName string `json:"pullSecretName,omitempty"`

	// Configs to be passed to subscription OLM object
	// +optional
	Config *SubscriptionConfig `json:"config,omitempty"`

	// Additional catalog source objects to be created in the cluster
	// +optional
	AdditionalCatalogSources []AdditionalCatalogSource `json:"additionalCatalogSources,omitempty"`
}

type SubscriptionConfig struct {
	// Array of env variables to be passed to the subscription object.
	EnvironmentVariables []EnvObject `json:"env"`
}

type AdditionalCatalogSource struct {
	// Name of the additional catalog source
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Image url of the additional catalog source
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
}

type EnvObject struct {
	// Name of the environment variable
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Value of the environment variable
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
}

// AllNamespaces specific Addon installation parameters.
type AddonInstallOLMAllNamespaces struct {
	AddonInstallOLMCommon `json:",inline"`
}

// OwnNamespace specific Addon installation parameters.
type AddonInstallOLMOwnNamespace struct {
	AddonInstallOLMCommon `json:",inline"`
}

type AddonInstallType string

const (
	// All namespaces on the cluster (default)
	// installs the Operator in the default openshift-operators namespace to
	// watch and be made available to all namespaces in the cluster.
	// Maps directly to the OLM default install mode "all namespaces".
	OLMAllNamespaces AddonInstallType = "OLMAllNamespaces"
	// Installs the operator into a specific namespace.
	// The Operator will only watch and be made available for use in this single namespace.
	// Maps directly to the OLM install mode "specific namespace"
	OLMOwnNamespace AddonInstallType = "OLMOwnNamespace"
)

// Addon condition reasons

const (
	// Addon as fully reconciled
	AddonReasonFullyReconciled = "FullyReconciled"

	// Addon is terminating
	AddonReasonTerminating = "Terminating"

	// Addon has a configurtion error
	AddonReasonConfigError = "ConfigurationError"

	// Addon has paused reconciliation
	AddonReasonPaused = "AddonPaused"

	// Addon has an unready Catalog source
	AddonReasonUnreadyCatalogSource = "UnreadyCatalogSource"

	// Addon has an unready additional Catalog source
	AddonReasonUnreadyAdditionalCatalogSource = "UnreadyAdditionalCatalogSource"

	// Addon has colliding namespaces
	AddonReasonCollidedNamespaces = "CollidedNamespaces"

	// Addon has unready namespaces
	AddonReasonUnreadyNamespaces = "UnreadyNamespaces"

	// Addon has unready metrics federation
	AddonReasonUnreadyMonitoring = "UnreadyMonitoringFederation"

	// Addon has unready CSV
	AddonReasonUnreadyCSV = "UnreadyCSV"

	// Addon cannot find a referenced secret to propagate
	AddonReasonMissingSecretForPropagation = "MissingSecretForPropagation"
)

type AddonNamespace struct {
	// Name of the KubernetesNamespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

const (
	// Available condition indicates that all resources for the Addon are reconciled and healthy
	Available = "Available"

	// Paused condition indicates that the reconciliation of resources for the Addon(s) has paused
	Paused = "Paused"
)

// AddonStatus defines the observed state of Addon
type AddonStatus struct {
	// The most recent generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions is a list of status conditions ths object is in.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// DEPRECATED: This field is not part of any API contract
	// it will go away as soon as kubectl can print conditions!
	// Human readable status - please use .Conditions from code
	Phase AddonPhase `json:"phase,omitempty"`
	// Tracks last reported upgrade policy status.
	// +optional
	UpgradePolicy *AddonUpgradePolicyStatus `json:"upgradePolicy,omitempty"`
	// Observed version of the Addon on the cluster, only present when .spec.version is populated.
	// +optional
	ObservedVersion string `json:"observedVersion,omitempty"`
}

type AddonPhase string

// Well-known Addon Phases for printing a Status in kubectl,
// see deprecation notice in AddonStatus for details.
const (
	PhasePending     AddonPhase = "Pending"
	PhaseReady       AddonPhase = "Ready"
	PhaseTerminating AddonPhase = "Terminating"
	PhaseError       AddonPhase = "Error"
)

// Addon is the Schema for the Addons API
//
// **Example**
// ```yaml
// apiVersion: addons.managed.openshift.io/v1alpha1
// kind: Addon
// metadata:
//   name: reference-addon
// spec:
//   displayName: An amazing example addon!
//   namespaces:
//   - name: reference-addon
//   install:
//     type: OLMOwnNamespace
//     olmOwnNamespace:
//       namespace: reference-addon
//       packageName: reference-addon
//       channel: alpha
//       catalogSourceImage: quay.io/osd-addons/reference-addon-index@sha256:58cb1c4478a150dc44e6c179d709726516d84db46e4e130a5227d8b76456b5bd
// ```
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Addon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AddonSpec `json:"spec,omitempty"`
	// +kubebuilder:default={phase:Pending}
	Status AddonStatus `json:"status,omitempty"`
}

// AddonList contains a list of Addon
// +kubebuilder:object:root=true
type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Addon `json:"items"`
}

func init() {
	register(&Addon{}, &AddonList{})
}

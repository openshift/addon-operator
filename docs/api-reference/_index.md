---
title: API Reference
weight: 50
---

# Addon Operator API Reference

The Addon Operator APIs are an extension of the [Kubernetes API](https://kubernetes.io/docs/reference/using-api/api-overview/) using `CustomResourceDefinitions`.

## `addons.managed.openshift.io`

The `addons.managed.openshift.io` API group in managed OpenShift contains all Addon related API objects.

	* [AddOnStatusCondition](#addonstatusconditionapimanagedopenshiftiov1alpha1)
	* [AdditionalCatalogSource](#additionalcatalogsourceapimanagedopenshiftiov1alpha1)
	* [Addon](#addonapimanagedopenshiftiov1alpha1)
	* [AddonInstallOLMAllNamespaces](#addoninstallolmallnamespacesapimanagedopenshiftiov1alpha1)
	* [AddonInstallOLMCommon](#addoninstallolmcommonapimanagedopenshiftiov1alpha1)
	* [AddonInstallOLMOwnNamespace](#addoninstallolmownnamespaceapimanagedopenshiftiov1alpha1)
	* [AddonInstallSpec](#addoninstallspecapimanagedopenshiftiov1alpha1)
	* [AddonList](#addonlistapimanagedopenshiftiov1alpha1)
	* [AddonNamespace](#addonnamespaceapimanagedopenshiftiov1alpha1)
	* [AddonPackageOperator](#addonpackageoperatorapimanagedopenshiftiov1alpha1)
	* [AddonSecretPropagation](#addonsecretpropagationapimanagedopenshiftiov1alpha1)
	* [AddonSecretPropagationReference](#addonsecretpropagationreferenceapimanagedopenshiftiov1alpha1)
	* [AddonSpec](#addonspecapimanagedopenshiftiov1alpha1)
	* [AddonStatus](#addonstatusapimanagedopenshiftiov1alpha1)
	* [AddonUpgradePolicy](#addonupgradepolicyapimanagedopenshiftiov1alpha1)
	* [AddonUpgradePolicyStatus](#addonupgradepolicystatusapimanagedopenshiftiov1alpha1)
	* [EnvObject](#envobjectapimanagedopenshiftiov1alpha1)
	* [MonitoringFederationSpec](#monitoringfederationspecapimanagedopenshiftiov1alpha1)
	* [MonitoringSpec](#monitoringspecapimanagedopenshiftiov1alpha1)
	* [MonitoringStackSpec](#monitoringstackspecapimanagedopenshiftiov1alpha1)
	* [OCMAddOnStatus](#ocmaddonstatusapimanagedopenshiftiov1alpha1)
	* [OCMAddOnStatusHash](#ocmaddonstatushashapimanagedopenshiftiov1alpha1)
	* [RHOBSRemoteWriteConfigSpec](#rhobsremotewriteconfigspecapimanagedopenshiftiov1alpha1)
	* [SubscriptionConfig](#subscriptionconfigapimanagedopenshiftiov1alpha1)
	* [AddonInstance](#addoninstanceapimanagedopenshiftiov1alpha1)
	* [AddonInstanceList](#addoninstancelistapimanagedopenshiftiov1alpha1)
	* [AddonInstanceSpec](#addoninstancespecapimanagedopenshiftiov1alpha1)
	* [AddonInstanceStatus](#addoninstancestatusapimanagedopenshiftiov1alpha1)
* [AddonOperator](#addonoperatorapimanagedopenshiftiov1alpha1)
	* [AddonOperatorFeatureToggles](#addonoperatorfeaturetogglesapimanagedopenshiftiov1alpha1)
	* [AddonOperatorOCM](#addonoperatorocmapimanagedopenshiftiov1alpha1)
	* [AddonOperatorSpec](#addonoperatorspecapimanagedopenshiftiov1alpha1)
	* [AddonOperatorStatus](#addonoperatorstatusapimanagedopenshiftiov1alpha1)
	* [ClusterSecretReference](#clustersecretreferenceapimanagedopenshiftiov1alpha1)

### AddOnStatusCondition.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| status_type |  | string | true |
| status_value |  | metav1.ConditionStatus | true |
| reason |  | string | true |
| message |  | string | true |

[Back to Group]()

### AdditionalCatalogSource.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| name | Name of the additional catalog source | string | true |
| image | Image url of the additional catalog source | string | true |

[Back to Group]()

### Addon.api.managed.openshift.io/v1alpha1

Addon is the Schema for the addons API

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta) | false |
| spec |  | [AddonSpec.api.managed.openshift.io/v1alpha1](#addonspecapimanagedopenshiftiov1alpha1) | false |
| status |  | [AddonStatus.api.managed.openshift.io/v1alpha1](#addonstatusapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### AddonInstallOLMAllNamespaces.api.managed.openshift.io/v1alpha1

AllNamespaces specific Addon installation parameters.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |

[Back to Group]()

### AddonInstallOLMCommon.api.managed.openshift.io/v1alpha1

Common Addon installation parameters.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| namespace | Namespace to install the Addon into. | string | true |
| catalogSourceImage | Defines the CatalogSource image. | string | true |
| channel | Channel for the Subscription object. | string | true |
| packageName | Name of the package to install via OLM. OLM will resove this package name to install the matching bundle. | string | true |
| pullSecretName | Reference to a secret of type kubernetes.io/dockercfg or kubernetes.io/dockerconfigjson in the addon operators installation namespace. The secret referenced here, will be made available to the addon in the addon installation namespace, as addon-pullsecret prior to installing the addon itself. | string | false |
| config | Configs to be passed to subscription OLM object | *[SubscriptionConfig.api.managed.openshift.io/v1alpha1](#subscriptionconfigapimanagedopenshiftiov1alpha1) | false |
| additionalCatalogSources | Additional catalog source objects to be created in the cluster | [][AdditionalCatalogSource.api.managed.openshift.io/v1alpha1](#additionalcatalogsourceapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### AddonInstallOLMOwnNamespace.api.managed.openshift.io/v1alpha1

OwnNamespace specific Addon installation parameters.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |

[Back to Group]()

### AddonInstallSpec.api.managed.openshift.io/v1alpha1

AddonInstallSpec defines the desired Addon installation type.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| type | Type of installation. | AddonInstallType.api.managed.openshift.io/v1alpha1 | true |
| olmAllNamespaces | OLMAllNamespaces config parameters. Present only if Type = OLMAllNamespaces. | *[AddonInstallOLMAllNamespaces.api.managed.openshift.io/v1alpha1](#addoninstallolmallnamespacesapimanagedopenshiftiov1alpha1) | false |
| olmOwnNamespace | OLMOwnNamespace config parameters. Present only if Type = OLMOwnNamespace. | *[AddonInstallOLMOwnNamespace.api.managed.openshift.io/v1alpha1](#addoninstallolmownnamespaceapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### AddonList.api.managed.openshift.io/v1alpha1

AddonList contains a list of Addon

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta) | false |
| items |  | [][Addon.api.managed.openshift.io/v1alpha1](#addonapimanagedopenshiftiov1alpha1) | true |

[Back to Group]()

### AddonNamespace.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| name | Name of the KubernetesNamespace. | string | true |
| labels | Labels to be added to the namespace | map[string]string | false |
| annotations | Annotations to be added to the namespace | map[string]string | false |

[Back to Group]()

### AddonPackageOperator.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| image |  | string | true |

[Back to Group]()

### AddonSecretPropagation.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| secrets |  | [][AddonSecretPropagationReference.api.managed.openshift.io/v1alpha1](#addonsecretpropagationreferenceapimanagedopenshiftiov1alpha1) | true |

[Back to Group]()

### AddonSecretPropagationReference.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| sourceSecret | Source secret name in the Addon Operator install namespace. | corev1.LocalObjectReference | true |
| destinationSecret | Destination secret name in every Addon namespace. | corev1.LocalObjectReference | true |

[Back to Group]()

### AddonSpec.api.managed.openshift.io/v1alpha1

AddonSpec defines the desired state of Addon

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| displayName | Human readable name for this addon. | string | true |
| version | Version of the Addon to deploy. Used for reporting via status and metrics. | string | false |
| pause | Pause reconciliation of Addon when set to True | bool | true |
| namespaces | Defines a list of Kubernetes Namespaces that belong to this Addon. Namespaces listed here will be created prior to installation of the Addon and will be removed from the cluster when the Addon is deleted. Collisions with existing Namespaces will result in the existing Namespaces being adopted. | [][AddonNamespace.api.managed.openshift.io/v1alpha1](#addonnamespaceapimanagedopenshiftiov1alpha1) | false |
| commonLabels | Labels to be applied to all resources. | map[string]string | false |
| commonAnnotations | Annotations to be applied to all resources. | map[string]string | false |
| correlationID | Correlation ID for co-relating current AddonCR revision and reported status. | string | false |
| install | Defines how an Addon is installed. This field is immutable. | [AddonInstallSpec.api.managed.openshift.io/v1alpha1](#addoninstallspecapimanagedopenshiftiov1alpha1) | true |
| deleteAckRequired | Defines whether the addon needs acknowledgment from the underlying addon's operator before deletion. | bool | true |
| installAckRequired | Defines if the addon needs installation acknowledgment from its corresponding addon instance. | bool | true |
| upgradePolicy | UpgradePolicy enables status reporting via upgrade policies. | *[AddonUpgradePolicy.api.managed.openshift.io/v1alpha1](#addonupgradepolicyapimanagedopenshiftiov1alpha1) | false |
| monitoring | Defines how an addon is monitored. | *[MonitoringSpec.api.managed.openshift.io/v1alpha1](#monitoringspecapimanagedopenshiftiov1alpha1) | false |
| secretPropagation | Settings for propagating secrets from the Addon Operator install namespace into Addon namespaces. | *[AddonSecretPropagation.api.managed.openshift.io/v1alpha1](#addonsecretpropagationapimanagedopenshiftiov1alpha1) | false |
| packageOperator | defines the PackageOperator image as part of the addon Spec | *[AddonPackageOperator.api.managed.openshift.io/v1alpha1](#addonpackageoperatorapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### AddonStatus.api.managed.openshift.io/v1alpha1

AddonStatus defines the observed state of Addon

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| observedGeneration | The most recent generation observed by the controller. | int64 | false |
| conditions | Conditions is a list of status conditions ths object is in. | []metav1.Condition | false |
| phase | DEPRECATED: This field is not part of any API contract it will go away as soon as kubectl can print conditions! Human readable status - please use .Conditions from code | AddonPhase.api.managed.openshift.io/v1alpha1 | false |
| upgradePolicy | Tracks last reported upgrade policy status. | *[AddonUpgradePolicyStatus.api.managed.openshift.io/v1alpha1](#addonupgradepolicystatusapimanagedopenshiftiov1alpha1) | false |
| ocmReportedStatusHash | Tracks the last addon status reported to OCM. | *[OCMAddOnStatusHash.api.managed.openshift.io/v1alpha1](#ocmaddonstatushashapimanagedopenshiftiov1alpha1) | false |
| observedVersion | Observed version of the Addon on the cluster, only present when .spec.version is populated. | string | false |
| lastObservedAvailableCSV | Namespaced name of the csv(available) that was last observed. | string | false |

[Back to Group]()

### AddonUpgradePolicy.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| id | Upgrade policy id. | string | true |

[Back to Group]()

### AddonUpgradePolicyStatus.api.managed.openshift.io/v1alpha1

Tracks the last state last reported to the Upgrade Policy endpoint.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| id | Upgrade policy id. | string | true |
| value | Upgrade policy value. | AddonUpgradePolicyValue.api.managed.openshift.io/v1alpha1 | true |
| version | Upgrade Policy Version. | string | false |
| observedGeneration | The most recent generation a status update was based on. | int64 | true |

[Back to Group]()

### EnvObject.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| name | Name of the environment variable | string | true |
| value | Value of the environment variable | string | true |

[Back to Group]()

### MonitoringFederationSpec.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| namespace | Namespace where the prometheus server is running. | string | true |
| portName | The name of the service port fronting the prometheus server. | string | true |
| matchNames | List of series names to federate from the prometheus server. | []string | true |
| matchLabels | List of labels used to discover the prometheus server(s) to be federated. | map[string]string | true |

[Back to Group]()

### MonitoringSpec.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| federation | Configuration parameters to be injected in the ServiceMonitor used for federation. The target prometheus server found by matchLabels needs to serve service-ca signed TLS traffic (https://docs.openshift.com/container-platform/4.6/security/certificate_types_descriptions/service-ca-certificates.html), and it needs to be running inside the namespace specified by `.monitoring.federation.namespace` with the service name 'prometheus'. | *[MonitoringFederationSpec.api.managed.openshift.io/v1alpha1](#monitoringfederationspecapimanagedopenshiftiov1alpha1) | false |
| monitoringStack | Settings For Monitoring Stack | *[MonitoringStackSpec.api.managed.openshift.io/v1alpha1](#monitoringstackspecapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### MonitoringStackSpec.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| rhobsRemoteWriteConfig | Settings for RHOBS Remote Write | *[RHOBSRemoteWriteConfigSpec.api.managed.openshift.io/v1alpha1](#rhobsremotewriteconfigspecapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### OCMAddOnStatus.api.managed.openshift.io/v1alpha1

Struct used to hash the reported addon status (along with correlationID).

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| addonID | ID of the addon. | string | true |
| correlationID | Correlation ID for co-relating current AddonCR revision and reported status. | string | true |
| version | Version of the addon | string | true |
| statusConditions | Reported addon status conditions | [][AddOnStatusCondition.api.managed.openshift.io/v1alpha1](#addonstatusconditionapimanagedopenshiftiov1alpha1) | true |
| observedGeneration | The most recent generation a status update was based on. | int64 | true |

[Back to Group]()

### OCMAddOnStatusHash.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| statusHash | Hash of the last reported status. | string | true |
| observedGeneration | The most recent generation a status update was based on. | int64 | true |

[Back to Group]()

### RHOBSRemoteWriteConfigSpec.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| url | RHOBS endpoints where your data is sent to It varies by environment: - Staging: https://observatorium-mst.stage.api.openshift.com/api/metrics/v1/<tenant id>/api/v1/receive - Production: https://observatorium-mst.api.openshift.com/api/metrics/v1/<tenant id>/api/v1/receive | string | true |
| oauth2 | OAuth2 config for the remote write URL | *monv1.OAuth2 | false |
| allowlist | List of metrics to push to RHOBS. Any metric not listed here is dropped. | []string | false |

[Back to Group]()

### SubscriptionConfig.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| env | Array of env variables to be passed to the subscription object. | [][EnvObject.api.managed.openshift.io/v1alpha1](#envobjectapimanagedopenshiftiov1alpha1) | true |

[Back to Group]()

### AddonInstance.api.managed.openshift.io/v1alpha1

AddonInstance is the Schema for the addoninstances API

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta) | false |
| spec |  | [AddonInstanceSpec.api.managed.openshift.io/v1alpha1](#addoninstancespecapimanagedopenshiftiov1alpha1) | false |
| status |  | [AddonInstanceStatus.api.managed.openshift.io/v1alpha1](#addoninstancestatusapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### AddonInstanceList.api.managed.openshift.io/v1alpha1

AddonInstanceList contains a list of AddonInstance

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#listmeta-v1-meta) | false |
| items |  | [][AddonInstance.api.managed.openshift.io/v1alpha1](#addoninstanceapimanagedopenshiftiov1alpha1) | true |

[Back to Group]()

### AddonInstanceSpec.api.managed.openshift.io/v1alpha1

AddonInstanceSpec defines the configuration to consider while taking AddonInstance-related decisions such as HeartbeatTimeouts

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| markedForDeletion | This field indicates whether the addon is marked for deletion. | bool | true |
| heartbeatUpdatePeriod | The periodic rate at which heartbeats are expected to be received by the AddonInstance object | metav1.Duration | false |

[Back to Group]()

### AddonInstanceStatus.api.managed.openshift.io/v1alpha1

AddonInstanceStatus defines the observed state of Addon

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| observedGeneration | The most recent generation observed by the controller. | int64 | false |
| conditions | Conditions is a list of status conditions ths object is in. | []metav1.Condition | false |
| lastHeartbeatTime | Timestamp of the last reported status check | metav1.Time | true |

[Back to Group]()

### AddonOperator.api.managed.openshift.io/v1alpha1

AddonOperator is the Schema for the AddonOperator API

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| metadata |  | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.22/#objectmeta-v1-meta) | false |
| spec |  | [AddonOperatorSpec.api.managed.openshift.io/v1alpha1](#addonoperatorspecapimanagedopenshiftiov1alpha1) | false |
| status |  | [AddonOperatorStatus.api.managed.openshift.io/v1alpha1](#addonoperatorstatusapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### AddonOperatorFeatureToggles.api.managed.openshift.io/v1alpha1



| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| experimentalFeatures | Feature toggle for enabling/disabling experimental features in the addon-operator | bool | true |

[Back to Group]()

### AddonOperatorOCM.api.managed.openshift.io/v1alpha1

OCM specific configuration.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| endpoint | Root of the OCM API Endpoint. | string | true |
| secret | Secret to authenticate to the OCM API Endpoint. Only supports secrets of type "kubernetes.io/dockerconfigjson" https://kubernetes.io/docs/concepts/configuration/secret/#secret-types | [ClusterSecretReference.api.managed.openshift.io/v1alpha1](#clustersecretreferenceapimanagedopenshiftiov1alpha1) | true |

[Back to Group]()

### AddonOperatorSpec.api.managed.openshift.io/v1alpha1

AddonOperatorSpec defines the desired state of Addon operator.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| pause | Pause reconciliation on all Addons in the cluster when set to True | bool | true |
| featureToggles | [DEPRECATED] Specification of the feature toggles supported by the addon-operator | [AddonOperatorFeatureToggles.api.managed.openshift.io/v1alpha1](#addonoperatorfeaturetogglesapimanagedopenshiftiov1alpha1) | true |
| featureFlags | Specification of the feature toggles supported by the addon-operator in the form of a comma-separated string | string | true |
| ocm | OCM specific configuration. Setting this subconfig will enable deeper OCM integration. e.g. push status reporting, etc. | *[AddonOperatorOCM.api.managed.openshift.io/v1alpha1](#addonoperatorocmapimanagedopenshiftiov1alpha1) | false |

[Back to Group]()

### AddonOperatorStatus.api.managed.openshift.io/v1alpha1

AddonOperatorStatus defines the observed state of Addon

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| observedGeneration | The most recent generation observed by the controller. | int64 | false |
| conditions | Conditions is a list of status conditions ths object is in. | []metav1.Condition | false |
| lastHeartbeatTime | Timestamp of the last reported status check | metav1.Time | true |
| phase | DEPRECATED: This field is not part of any API contract it will go away as soon as kubectl can print conditions! Human readable status - please use .Conditions from code | AddonPhase.api.managed.openshift.io/v1alpha1 | false |

[Back to Group]()

### ClusterSecretReference.api.managed.openshift.io/v1alpha1

References a secret on the cluster.

| Field | Description | Scheme | Required |
| ----- | ----------- | ------ | -------- |
| name | Name of the secret object. | string | true |
| namespace | Namespace of the secret object. | string | true |

[Back to Group]()

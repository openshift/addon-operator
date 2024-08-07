package controllers

import "errors"

type ControllerReconcileError struct {
	Reason string
}

func newControllerReconcileError(reason string) *ControllerReconcileError {
	return &ControllerReconcileError{
		Reason: reason,
	}
}

func (c *ControllerReconcileError) Error() string {
	return c.Reason
}

var (
	// This error is returned when a reconciled child object already
	// exists and is not owned by the current controller/addon
	ErrNotOwnedByUs = errors.New("object is not owned by us")

	// Failed to get an addon
	ErrGetAddon = newControllerReconcileError("err_get_addon")
	// An error happened while syncing with external APIs
	ErrSyncWithExternalAPIs = newControllerReconcileError("err_sync_with_external_apis")
	// An OCM client request error was encountered
	ErrOCMClientRequest = newControllerReconcileError("err_ocm_client_request")
	// Failed to update an addon
	ErrUpdateAddon = newControllerReconcileError("err_update_addon")
	// Failed to notify addon
	ErrNotifyAddon = newControllerReconcileError("err_notify_addon")
	// Failed to receive ack from addon
	ErrAckReceivedFromAddon = newControllerReconcileError("err_ack_received_from_addon")
	// Failed to ensure creation of addoninstance
	ErrEnsureCreateAddonInstance = newControllerReconcileError("err_ensure_create_addoninstance")
	// Failed to ensure creation of servicemonitor
	ErrEnsureCreateServiceMonitor = newControllerReconcileError("err_ensure_servicemonitor")
	// Failed to ensure deletion of servicemonitor
	ErrEnsureDeleteServiceMonitor = newControllerReconcileError("err_ensure_delete_servicemonitor")
	// Failed to ensure creation of monitoringstack
	ErrEnsureCreateMonitoringStack = newControllerReconcileError("err_ensure_create_monitoringstack")
	// Failed to ensure creation of namespace
	ErrEnsureCreateNamespace = newControllerReconcileError("err_ensure_create_namespace")
	// Failed to ensure deletion of namespace
	ErrEnsureDeleteNamespace = newControllerReconcileError("err_ensure_delete_namespace")
	// Failed to ensure existence of operator group
	ErrEnsureOperatorGroup = newControllerReconcileError("err_ensure_operator_group")
	// Failed to ensure existence of networkpolicy
	ErrEnsureNetworkPolicy = newControllerReconcileError("err_ensure_networkpolicy")
	// Failed to ensure existence of catalogsource
	ErrEnsureCatalogSource = newControllerReconcileError("err_ensure_catalogsource")
	// Failed to ensure existence of additional catalogsource
	ErrEnsureAdditionalCatalogSource = newControllerReconcileError("err_ensure_additional_catalogsource")
	// An error happened while reconciling a subscription
	ErrReconcileSubscription = newControllerReconcileError("err_reconcile_subscription")
	// An error happened while observing a CSV
	ErrObserveCSV = newControllerReconcileError("err_observe_csv")
	// Failed to ensure deletion of clusterobjecttemplate
	ErrEnsureDeleteClusterObjectTemplate = newControllerReconcileError("err_ensure_delete_of_clusterobjecttemplate")
	// An error happened while reconcileing clusterobjecttemplate
	ErrReconcileClusterObjectTemplate = newControllerReconcileError("err_reconcile_cluster_object_template")
	// Failed to cleanup unknown secrets
	ErrCleanupUnknownSecrets = newControllerReconcileError("err_cleanup_unknown_secrets")
	// Failed to get target/destination secrets that didn't have namespace
	ErrGetDestinationSecretsWithoutNamespace = newControllerReconcileError("err_get_destination_secrets_without_namespace")
	// Failed reconcile secrets in addon namespaces
	ErrReconcileSecretsInAddonNamespaces = newControllerReconcileError("err_reconcile_secrets_in_addon_namespaces")
	// Failed to get addoninstance
	ErrGetAddonInstance = newControllerReconcileError("err_get_addoninstance")
	// Failed to update addoninstance status
	ErrUpdateAddonInstanceStatus = newControllerReconcileError("err_update_addon_instance_status")
	// Failed to execute addoninstance reconcile phase
	ErrExecuteAddonInstanceReconcilePhase = newControllerReconcileError("err_execute_addon_instance_reconcile_phase")
	// Failed to get default addonoperator
	ErrGetDefaultAddonOperator = newControllerReconcileError("err_get_default_addon_operator")
	// Failed to create addonoperator
	ErrCreateAddonOperator = newControllerReconcileError("err_create_addon_operator")
	// Failed to handle global pause of addon-operator
	ErrAddonOperatorHandleGlobalPause = newControllerReconcileError("err_addon_operator_handle_global_pause")
	// Failed to create OCM client
	ErrCreateOCMClient = newControllerReconcileError("err_create_ocm_client")
	// Failed to report addon-operator readiness status
	ErrReportAddonOperatorStatus = newControllerReconcileError("err_report_addonoperator_status")
)

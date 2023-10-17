package controllers

import (
	"errors"
)

var (
	// This error is returned when a reconciled child object already
	// exists and is not owned by the current controller/addon
	ErrNotOwnedByUs = errors.New("object is not owned by us")

	// Reconciler/Controller reconcile errors
	ErrAddonNotFound                         = errors.New("failed to get addon")
	ErrAddFinalizer                          = errors.New("failed to add finalizer")
	ErrNotifyAddon                           = errors.New("failed to notify addon")
	ErrAckReceivedFromAddon                  = errors.New("failed to receive ack from addon")
	ErrEnsureCreateAddonInstance             = errors.New("failed to ensure creation of addoninstance")
	ErrEnsureCreateServiceMonitor            = errors.New("failed to ensure servicemonitor")
	ErrEnsureDeleteUnwantedServiceMonitor    = errors.New("failed to ensure deletion of unwanted servicemonitor")
	ErrEnsureCreateMonitoringStack           = errors.New("failed to ensure creation of monitoring stack")
	ErrEnsureCreateNamespaces                = errors.New("failed to ensure creation of namespaces")
	ErrEnsureDeleteNamespaces                = errors.New("failed to ensure deletion of namespaces")
	ErrEnsureOperatorGroup                   = errors.New("failed to ensure operatorgroup")
	ErrEnsureNetworkPolicy                   = errors.New("failed to ensure networkpolicy for catalgosources")
	ErrEnsureCatalogSource                   = errors.New("failed to ensure catalogsource")
	ErrEnsureAdditionalCatalogSource         = errors.New("failed to ensure additional catalogsource")
	ErrEnsureSubscription                    = errors.New("failed to ensure subscription")
	ErrObserveCurrentCSV                     = errors.New("failed to observe current CSV")
	ErrEnsureClusterObjectTemplateTornDown   = errors.New("failed to ensure tearing down of clusterobjecttemplate")
	ErrEnsureClusterObjectTemplate           = errors.New("failed to ensure clusterobjecttemplate")
	ErrCleanupUnknownSecrets                 = errors.New("failed to cleanup unknown secrets")
	ErrGetDestinationSecretsWithoutNamespace = errors.New("failed to get destination secrets without namespace")
	ErrEnsureSecretsInAddonNamespaces        = errors.New("failed to ensure serects in addon namespaces")
	ErrCleanupUnkownSecrets                  = errors.New("failed to cleanup unknown secrets")
	ErrAddonInstanceNotFound                 = errors.New("failed to of get addoninstance")
	ErrUpdatingAddonInstanceStatus           = errors.New("failed to update addoninstance status")
	ErrExecuteAddonInstanceReconcilePhase    = errors.New("failed to execute addoninstance reconcile phase")
	ErrDefaultAddonOperatorNotFound          = errors.New("failed to find default addonoperators")
	ErrCreateAddonOperator                   = errors.New("failed to create addonoperator")
	ErrAddonOperatorHandleGlobalPause        = errors.New("failed to handle addonoperator global pause")
	ErrCreateOCMClient                       = errors.New("failed to create OCM client")
	ErrReportAddonOperatorReadinessStatus    = errors.New("failed to report addonoperator readiness status")
)

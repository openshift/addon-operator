package addon

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
)

// use this type for exit handling
type requeueResult int

const (
	// Should be used when requeue result does not matter.
	// For example, when an error is returned along with it.
	resultNil requeueResult = iota

	// Should be used when request needs to be retried
	resultRetry

	// Should be used when reconciler needs to stop and exit.
	resultStop
)

// This method should be called ONLY if result is NOT `resultNil`, or it could
// lead to unpredictable behaviour.
func handleExit(result requeueResult) ctrl.Result {
	switch result {
	case resultRetry:
		return ctrl.Result{
			RequeueAfter: defaultRetryAfterTime,
		}
	default:
		return ctrl.Result{}
	}
}

func markedForDeletion(addon *addonsv1alpha1.Addon) bool {
	_, found := addon.Annotations[addonsv1alpha1.DeleteAnnotationFlag]
	return found
}

// Handle the deletion of an AddonCR.
func (r *AddonReconciler) handleAddonCRDeletion(
	ctx context.Context, addon *addonsv1alpha1.Addon,
) (res ctrl.Result, err error) {
	if !controllerutil.ContainsFinalizer(addon, cacheFinalizer) {
		// The finalizer is already gone and the deletion timestamp is set.
		// kube-apiserver should have garbage collected this object already,
		// this delete signal does not need further processing.
		return res, nil
	}

	// Report that Addon is deleting.
	reportTerminationStatus(addon)
	if err := r.Status().Update(ctx, addon); err != nil {
		return res, fmt.Errorf("updating Addon status: %w", err)
	}

	res, err = r.ensureClusterPackageDeletion(ctx, addon)
	if err != nil {
		return res, fmt.Errorf("ensure ClusterPackage deletion: %w", err)
	}
	if !res.IsZero() {
		return res, nil
	}

	// Clear from CSV Event Handler
	r.operatorResourceHandler.Free(addon)

	controllerutil.RemoveFinalizer(addon, cacheFinalizer)
	if err := r.Update(ctx, addon); err != nil {
		return res, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	return res, nil
}

func (r *AddonReconciler) ensureClusterPackageDeletion(
	ctx context.Context, addon *addonsv1alpha1.Addon,
) (res ctrl.Result, err error) {
	cot := &pkov1alpha1.ClusterObjectTemplate{}
	cot.SetName(addon.Name)
	err = r.Delete(ctx, cot)
	if err == nil {
		res.RequeueAfter = defaultRetryAfterTime
		return res, nil
	}
	if errors.IsNotFound(err) {
		return res, nil
	}
	return res, err
}

// Report Addon status to communicate that everything is alright
func reportReadinessStatus(addon *addonsv1alpha1.Addon) {
	meta.SetStatusCondition(&addon.Status.Conditions, metav1.Condition{
		Type:               addonsv1alpha1.Available,
		Status:             metav1.ConditionTrue,
		Message:            "All components are ready.",
		Reason:             addonsv1alpha1.AddonReasonFullyReconciled,
		ObservedGeneration: addon.Generation,
	})
	addon.Status.ObservedGeneration = addon.Generation
	addon.Status.Phase = addonsv1alpha1.PhaseReady

	// When everything is ready, we are also operating on the current version of the Addon.
	// Otherwise we would be in a pending or error state.
	addon.Status.ObservedVersion = addon.Spec.Version
}

func reportObservedVersion(addon *addonsv1alpha1.Addon) {
	// When everything is ready, we are also operating on the current version of the Addon.
	// Otherwise we would be in a pending or error state.
	addon.Status.ObservedVersion = addon.Spec.Version
}

// Report Addon status to communicate that the Addon is terminating
func reportTerminationStatus(addon *addonsv1alpha1.Addon) {
	meta.SetStatusCondition(&addon.Status.Conditions, metav1.Condition{
		Type:               addonsv1alpha1.Available,
		Status:             metav1.ConditionFalse,
		Reason:             addonsv1alpha1.AddonReasonTerminating,
		ObservedGeneration: addon.Generation,
	})
	addon.Status.ObservedGeneration = addon.Generation
	addon.Status.Phase = addonsv1alpha1.PhaseTerminating
}

// Report Addon status to communicate that the resource is misconfigured
func reportConfigurationError(addon *addonsv1alpha1.Addon, message string) {
	meta.SetStatusCondition(&addon.Status.Conditions, metav1.Condition{
		Type:    addonsv1alpha1.Available,
		Status:  metav1.ConditionFalse,
		Reason:  addonsv1alpha1.AddonReasonConfigError,
		Message: message,
	})
	addon.Status.ObservedGeneration = addon.Generation
	addon.Status.Phase = addonsv1alpha1.PhaseError
}

// Marks Addon as paused
func reportAddonPauseStatus(addon *addonsv1alpha1.Addon,
	reason string) {
	meta.SetStatusCondition(&addon.Status.Conditions, metav1.Condition{
		Type:               addonsv1alpha1.Paused,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            "",
		ObservedGeneration: addon.Generation,
	})
	addon.Status.ObservedGeneration = addon.Generation
}

func reportAddonReadyToBeDeletedStatus(addon *addonsv1alpha1.Addon, value metav1.ConditionStatus) {
	if value == metav1.ConditionTrue {
		meta.SetStatusCondition(&addon.Status.Conditions, metav1.Condition{
			Type:               addonsv1alpha1.ReadyToBeDeleted,
			Status:             value,
			Reason:             addonsv1alpha1.AddonReasonReadyToBeDeleted,
			Message:            "Addon is ready to be deleted.",
			ObservedGeneration: addon.Generation,
		})
		addon.Status.ObservedGeneration = addon.Generation
	} else {
		meta.SetStatusCondition(&addon.Status.Conditions, metav1.Condition{
			Type:               addonsv1alpha1.ReadyToBeDeleted,
			Status:             value,
			Reason:             addonsv1alpha1.AddonReasonNotReadyToBeDeleted,
			Message:            "Addon is not yet ready to be deleted.",
			ObservedGeneration: addon.Generation,
		})
		addon.Status.ObservedGeneration = addon.Generation
	}
}

func reportAddonDeletionTimedOut(addon *addonsv1alpha1.Addon) {
	meta.SetStatusCondition(&addon.Status.Conditions, metav1.Condition{
		Type:               addonsv1alpha1.DeleteTimeout,
		Status:             metav1.ConditionTrue,
		Reason:             addonsv1alpha1.AddonReasonDeletionTimedOut,
		Message:            "Addon deletion has timed out.",
		ObservedGeneration: addon.Generation,
	})
	addon.Status.ObservedGeneration = addon.Generation
}

// remove Paused condition from Addon
func (r *AddonReconciler) removeAddonPauseCondition(addon *addonsv1alpha1.Addon) {
	meta.RemoveStatusCondition(&addon.Status.Conditions, addonsv1alpha1.Paused)
	addon.Status.ObservedGeneration = addon.Generation
}

func reportLastObservedAvailableCSV(addon *addonsv1alpha1.Addon, csvName string) {
	addon.Status.LastObservedAvailableCSV = csvName
}

func reportAddonUpgradeSucceeded(addon *addonsv1alpha1.Addon) {
	upgradeStartedCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.UpgradeStarted)
	// Only set upgrade condition to succeeded, if UpgradeStarted condition is already present.
	if upgradeStartedCond != nil {
		// Remove the upgrade started condition
		meta.RemoveStatusCondition(&addon.Status.Conditions, addonsv1alpha1.UpgradeStarted)
		meta.SetStatusCondition(&addon.Status.Conditions,
			metav1.Condition{
				Type:               addonsv1alpha1.UpgradeSucceeded,
				Status:             metav1.ConditionTrue,
				Reason:             addonsv1alpha1.AddonReasonUpgradeSucceeded,
				Message:            "Addon upgrade has succeeded.",
				ObservedGeneration: addon.Generation,
			})
		addon.Status.ObservedGeneration = addon.Generation
	}
}

func reportAddonUpgradeStarted(addon *addonsv1alpha1.Addon) {
	// If upgrade succeeded status was previously set, remove it.
	upgradeSucceededCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.UpgradeSucceeded)
	if upgradeSucceededCond != nil {
		meta.RemoveStatusCondition(&addon.Status.Conditions, addonsv1alpha1.UpgradeSucceeded)
	}
	meta.SetStatusCondition(&addon.Status.Conditions,
		metav1.Condition{
			Type:               addonsv1alpha1.UpgradeStarted,
			Status:             metav1.ConditionTrue,
			Reason:             addonsv1alpha1.AddonReasonUpgradeStarted,
			Message:            "Addon upgrade has started.",
			ObservedGeneration: addon.Generation,
		})
	addon.Status.ObservedGeneration = addon.Generation
}

func reportUninstalledCondition(addon *addonsv1alpha1.Addon) {
	installedCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Installed)
	if installedCond != nil {
		meta.SetStatusCondition(&addon.Status.Conditions,
			metav1.Condition{
				Type:               addonsv1alpha1.Installed,
				Status:             metav1.ConditionFalse,
				Reason:             addonsv1alpha1.AddonReasonNotInstalled,
				Message:            "Addon has been uninstalled.",
				ObservedGeneration: addon.Generation,
			},
		)
		addon.Status.ObservedGeneration = addon.Generation
	}
}

func reportInstalledCondition(addon *addonsv1alpha1.Addon) {
	meta.SetStatusCondition(&addon.Status.Conditions,
		metav1.Condition{
			Type:               addonsv1alpha1.Installed,
			Status:             metav1.ConditionTrue,
			Reason:             addonsv1alpha1.AddonReasonInstalled,
			Message:            "Addon has been successfully installed.",
			ObservedGeneration: addon.Generation,
		},
	)
	addon.Status.ObservedGeneration = addon.Generation
}

func addonUpgradeStarted(addon *addonsv1alpha1.Addon) bool {
	upgradeStartedCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.UpgradeStarted)
	if upgradeStartedCond != nil {
		return upgradeStartedCond.Status == metav1.ConditionTrue
	}
	return false
}

func addonIsBeingUpgraded(addon *addonsv1alpha1.Addon) bool {
	if len(addon.Spec.Version) != 0 &&
		len(addon.Status.ObservedVersion) != 0 {
		return addon.Spec.Version != addon.Status.ObservedVersion
	}
	return false
}

func installedConditionMissing(addon *addonsv1alpha1.Addon) bool {
	cond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Installed)
	return cond == nil
}

func reportInstalledConditionFalse(addon *addonsv1alpha1.Addon) {
	meta.SetStatusCondition(&addon.Status.Conditions,
		metav1.Condition{
			Type:               addonsv1alpha1.Installed,
			Status:             metav1.ConditionFalse,
			Reason:             addonsv1alpha1.AddonReasonNotInstalled,
			Message:            "Addon is not yet installed.",
			ObservedGeneration: addon.Generation,
		},
	)
	addon.Status.ObservedGeneration = addon.Generation
}

// Marks Addon as unavailable because the CatalogSource is unready
func reportCatalogSourceUnreadinessStatus(addon *addonsv1alpha1.Addon, message string) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonUnreadyCatalogSource,
		fmt.Sprintf("CatalogSource connection is not ready: %s", message))
}

func reportAdditionalCatalogSourceUnreadinessStatus(addon *addonsv1alpha1.Addon, message string) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonUnreadyAdditionalCatalogSource,
		fmt.Sprintf("CatalogSource connection is not ready: %s", message))
}

func reportUnreadyNamespaces(addon *addonsv1alpha1.Addon, unreadyNamespaces []string) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonUnreadyNamespaces,
		fmt.Sprintf("Namespaces not yet in Active phase: %s", strings.Join(unreadyNamespaces, ", ")))
}

func reportUnreadyCSV(addon *addonsv1alpha1.Addon, message string) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonUnreadyCSV,
		fmt.Sprintf("ClusterServiceVersion is not ready: %s", message))
}

func reportMissingCSV(addon *addonsv1alpha1.Addon) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonMissingCSV, "ClusterServiceVersion is missing.")
}

func reportInstallPlanPending(addon *addonsv1alpha1.Addon) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonInstallPlanPending, "InstallPlan is waiting for approval.")
}

func reportPendingAddonInstanceInstallation(addon *addonsv1alpha1.Addon) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonInstanceNotInstalled, "Addon instance is not yet installed.")
}

func reportUnreadyMonitoringFederation(addon *addonsv1alpha1.Addon, message string) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonUnreadyMonitoringFederation,
		fmt.Sprintf("Monitoring Federation is not ready: %s", message))
}

func reportUnreadyMonitoringStack(addon *addonsv1alpha1.Addon, message string) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonUnreadyMonitoringStack,
		fmt.Sprintf("MonitoringStack is not ready: %s", message))
}

func reportUnreadyClusterObjectTemplate(addon *addonsv1alpha1.Addon) {
	reportPendingStatus(addon, addonsv1alpha1.AddonReasonUnreadyClusterPackageTemplate,
		"PackageOperator ClusterPackageTemplate is not ready")
}

func reportPendingStatus(addon *addonsv1alpha1.Addon, reason, msg string) {
	meta.SetStatusCondition(&addon.Status.Conditions,
		metav1.Condition{
			Type:               addonsv1alpha1.Available,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            msg,
			ObservedGeneration: addon.Generation,
		})

	addon.Status.ObservedGeneration = addon.Generation
	addon.Status.Phase = addonsv1alpha1.PhasePending
}

// Validate addon.Spec.Install then extract
// targetNamespace and catalogSourceImage from it
func parseAddonInstallConfig(
	log logr.Logger, addon *addonsv1alpha1.Addon) (
	common *addonsv1alpha1.AddonInstallOLMCommon, stop bool,
) {
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMOwnNamespace:
		if addon.Spec.Install.OLMOwnNamespace == nil ||
			len(addon.Spec.Install.OLMOwnNamespace.Namespace) == 0 {
			// invalid/missing configuration
			reportConfigurationError(addon,
				".spec.install.ownNamespace.namespace is required when .spec.install.type = OwnNamespace")
			return nil, true
		}

		if len(addon.Spec.Install.OLMOwnNamespace.CatalogSourceImage) == 0 {
			// invalid/missing configuration
			reportConfigurationError(addon,
				".spec.install.ownNamespace.catalogSourceImage is"+
					"required when .spec.install.type = OwnNamespace")
			return nil, true
		}
		return &addon.Spec.Install.OLMOwnNamespace.AddonInstallOLMCommon, false

	case addonsv1alpha1.OLMAllNamespaces:
		if addon.Spec.Install.OLMAllNamespaces == nil ||
			len(addon.Spec.Install.OLMAllNamespaces.Namespace) == 0 {
			// invalid/missing configuration
			reportConfigurationError(addon,
				".spec.install.allNamespaces.namespace is required when"+
					" .spec.install.type = AllNamespaces")
			return nil, true
		}

		if len(addon.Spec.Install.OLMAllNamespaces.CatalogSourceImage) == 0 {
			// invalid/missing configuration
			reportConfigurationError(addon,
				".spec.install.allNamespaces.catalogSourceImage is required"+
					"when .spec.install.type = AllNamespaces")
			return nil, true
		}

		return &addon.Spec.Install.OLMAllNamespaces.AddonInstallOLMCommon, false

	default:
		// Unsupported Install Type
		// This should never happen, unless the schema validation is wrong.
		// The .install.type property is set to only allow known enum values.
		log.Error(fmt.Errorf("invalid Addon install type: %q", addon.Spec.Install.Type),
			"stopping Addon reconcilation")
		return nil, true
	}
}

func parseAddonInstallConfigForAdditionalCatalogSources(
	log logr.Logger, addon *addonsv1alpha1.Addon) (
	additionalCatalogSrcs []addonsv1alpha1.AdditionalCatalogSource,
	targetNamespace string,
	pullSecretName string,
	stop bool) {
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMOwnNamespace:
		for _, additionalCatalogSrc := range addon.Spec.Install.OLMOwnNamespace.AdditionalCatalogSources {
			if len(additionalCatalogSrc.Image) == 0 || len(additionalCatalogSrc.Name) == 0 {
				reportConfigurationError(addon,
					".spec.install.ownNamespace.additionalCatalogSources"+
						"requires both image and name")
				return []addonsv1alpha1.AdditionalCatalogSource{}, "", "", true
			}
			additionalCatalogSrcs = append(additionalCatalogSrcs, additionalCatalogSrc)
		}
		targetNamespace = addon.Spec.Install.OLMOwnNamespace.Namespace
		pullSecretName = addon.Spec.Install.OLMOwnNamespace.PullSecretName
	case addonsv1alpha1.OLMAllNamespaces:
		for _, additionalCatalogSrc := range addon.Spec.Install.OLMAllNamespaces.AdditionalCatalogSources {
			if len(additionalCatalogSrc.Image) == 0 || len(additionalCatalogSrc.Name) == 0 {
				reportConfigurationError(addon,
					".spec.install.allNamespaces.additionalCatalogSources"+
						"requires both image and name")
				return []addonsv1alpha1.AdditionalCatalogSource{}, "", "", true
			}
			additionalCatalogSrcs = append(additionalCatalogSrcs, additionalCatalogSrc)
		}
		targetNamespace = addon.Spec.Install.OLMAllNamespaces.Namespace
		pullSecretName = addon.Spec.Install.OLMAllNamespaces.PullSecretName
	default:
		// Unsupported Install Type
		// This should never happen, unless the schema validation is wrong.
		// The .install.type property is set to only allow known enum values.
		log.Error(fmt.Errorf("invalid Addon install type: %q", addon.Spec.Install.Type),
			"stopping Addon reconcilation")
		return []addonsv1alpha1.AdditionalCatalogSource{}, "", "", true
	}
	return additionalCatalogSrcs, targetNamespace, pullSecretName, false
}

// HasMonitoringFederation is a helper to determine if a given addon's spec
// defines a Monitoring.Federation.
func HasMonitoringFederation(addon *addonsv1alpha1.Addon) bool {
	return addon.Spec.Monitoring != nil && addon.Spec.Monitoring.Federation != nil
}

// HasMonitoringStack is a helper to determine if a given addon's spec
// defines a Monitoring.Stack.
func HasMonitoringStack(addon *addonsv1alpha1.Addon) bool {
	return addon.Spec.Monitoring != nil && addon.Spec.Monitoring.MonitoringStack != nil
}

// HasAdditionalCatalogSources determines whether the passed addon's spec
// contains additional catalog sources
func HasAdditionalCatalogSources(addon *addonsv1alpha1.Addon) bool {
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMOwnNamespace:
		return len(addon.Spec.Install.OLMOwnNamespace.AdditionalCatalogSources) > 0
	case addonsv1alpha1.OLMAllNamespaces:
		return len(addon.Spec.Install.OLMAllNamespaces.AdditionalCatalogSources) > 0
	default:
		return false
	}
}

// Helper function to compute monitoring Namespace name from addon object
func GetMonitoringNamespaceName(addon *addonsv1alpha1.Addon) string {
	return fmt.Sprintf("redhat-monitoring-%s", addon.Name)
}

// Helper function to compute monitoring federation ServiceMonitor name from addon object
func GetMonitoringFederationServiceMonitorName(addon *addonsv1alpha1.Addon) string {
	return fmt.Sprintf("federated-sm-%s", addon.Name)
}

// GetMonitoringFederationServiceMonitorEndpoints generates a slice of monitoringv1.Endpoint
// instances from an addon's Monitoring.Federation specification.
func GetMonitoringFederationServiceMonitorEndpoints(addon *addonsv1alpha1.Addon, bearertokensecret *corev1.Secret) []monitoringv1.Endpoint {
	const cacert = "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt"

	tlsConfig := &monitoringv1.TLSConfig{
		CAFile: cacert,
		SafeTLSConfig: monitoringv1.SafeTLSConfig{
			ServerName: fmt.Sprintf("prometheus.%s.svc", addon.Spec.Monitoring.Federation.Namespace),
		},
	}

	matchParams := []string{`ALERTS{alertstate="firing"}`}

	for _, name := range addon.Spec.Monitoring.Federation.MatchNames {
		matchParams = append(matchParams, fmt.Sprintf(`{__name__="%s"}`, name))
	}
	auth := &monitoringv1.SafeAuthorization{Type: "Bearer", Credentials: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: bearertokensecret.Name}, Key: "token"}}

	log.Println("the Auth struct ", auth)
	return []monitoringv1.Endpoint{{
		Authorization: auth,
		HonorLabels:   true,
		Port:          addon.Spec.Monitoring.Federation.PortName,
		Path:          "/federate",
		Scheme:        "https",
		Interval:      "30s",
		TLSConfig:     tlsConfig,
		Params:        map[string][]string{"match[]": matchParams},
	}}
}

func getPrimaryCatalogSourceName(addon *addonsv1alpha1.Addon) string {
	return fmt.Sprintf("addon-%s-catalog", addon.Name)
}

func getCatalogSourceNetworkPolicyName(addon *addonsv1alpha1.Addon) string {
	return fmt.Sprintf("addon-%s-catalogs", addon.Name)
}

func CatalogSourceName(addon *addonsv1alpha1.Addon) string {
	return fmt.Sprintf("addon-%s-catalog", addon.Name)
}

func SubscriptionName(addon *addonsv1alpha1.Addon) string {
	return fmt.Sprintf("addon-%s", addon.Name)
}

func GetCommonInstallOptions(addon *addonsv1alpha1.Addon) (commonInstallOptions addonsv1alpha1.AddonInstallOLMCommon) {
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMAllNamespaces:
		commonInstallOptions = addon.Spec.Install.
			OLMAllNamespaces.AddonInstallOLMCommon
	case addonsv1alpha1.OLMOwnNamespace:
		commonInstallOptions = addon.Spec.Install.
			OLMOwnNamespace.AddonInstallOLMCommon
	}
	return
}

func corev1ProtocolPtr(proto corev1.Protocol) *corev1.Protocol   { return &proto }
func intOrStringPtr(iors intstr.IntOrString) *intstr.IntOrString { return &iors }

func HashCurrentAddonStatus(addon *addonsv1alpha1.Addon) string {
	ocmAddonStatus := addonsv1alpha1.OCMAddOnStatus{
		AddonID:          addon.Name,
		CorrelationID:    addon.Spec.CorrelationID,
		AddonVersion:     addon.Spec.Version,
		StatusConditions: mapToAddonStatusConditions(addon.Status.Conditions),
	}
	return hashOCMAddonStatus(ocmAddonStatus)
}

func hashOCMAddonStatus(ocmAddonStatus addonsv1alpha1.OCMAddOnStatus) string {
	hasher := fnv.New32a()
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	printer.Fprintf(hasher, "%#v", ocmAddonStatus)
	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}

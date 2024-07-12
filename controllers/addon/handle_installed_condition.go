package addon

import (
	"context"
	"fmt"
	"strings"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
)

func (r *olmReconciler) handleInstalledCondition(ctx context.Context,
	addon *addonsv1alpha1.Addon, addonCSVRef *operatorsv1.RichReference) (requeueResult, error) {
	// Handle missing CSV, addon might have been uninstalled?.
	if addonCSVRef == nil {
		return r.handleMissingCSV(ctx, addon)
	}

	// Fetch the addon's current csv phase
	csvPhase := getCSVPhase(addonCSVRef)

	// If csv is in the succeeded phase we report the addon as installed.
	if csvPhase == operatorsv1alpha1.CSVPhaseSucceeded {
		// If addon's operator choose to onboard addon instance into their workflow.
		// Wait for the addon instance installation acknowledgement.
		if addon.Spec.InstallAckRequired {
			return r.handleInstallAck(ctx, addon)
		}

		reportInstalledCondition(addon)
	}

	return resultNil, nil
}

func (r *olmReconciler) IsPKOFailed(addon *addonsv1alpha1.Addon) bool {
	availableCondition := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Available)
	if availableCondition != nil && strings.Contains(availableCondition.Message, "PackageOperator") {
		return true
	}
	return false
}

func (r *olmReconciler) handleMissingCSV(ctx context.Context, addon *addonsv1alpha1.Addon) (requeueResult, error) {
	// Since CSV is missing, we report the addon as unavailable.
	reportMissingCSV(addon)
	// Check if the addon removed its CSV as part of the uninstallation
	// process by looking for the delete configmap created.
	found, err := r.deleteConfigMapPresent(ctx, addon)
	if err != nil {
		return resultRetry, err
	}
	if found {
		reportUninstalledCondition(addon)
		return resultStop, nil
	}
	// If delete configmap is not present but the CSV is missing
	// we want to requeue this request since we dont watch configmaps.
	return resultRetry, nil
}

func (r *olmReconciler) deleteConfigMapPresent(ctx context.Context, addon *addonsv1alpha1.Addon) (bool, error) {
	deleteConfigMapLabel := fmt.Sprintf("api.openshift.com/addon-%v-delete", addon.Name)
	deleteConfigMap := &corev1.ConfigMap{}
	addonNamespace := GetCommonInstallOptions(addon).Namespace
	err := r.uncachedClient.Get(ctx, types.NamespacedName{
		Name:      addon.Name,
		Namespace: addonNamespace,
	}, deleteConfigMap)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	_, found := deleteConfigMap.Labels[deleteConfigMapLabel]
	return found, nil
}

// handleInstallAck handles installation acknowledgement from
// the addon instance if specified in the addon CR
func (r *olmReconciler) handleInstallAck(ctx context.Context, addon *addonsv1alpha1.Addon) (requeueResult, error) {
	installed, err := r.isAddonInstanceInstalled(ctx, addon)
	if err != nil {
		return resultNil, err
	}

	if !installed {
		return resultRetry, nil
	}

	reportInstalledCondition(addon)
	return resultNil, nil
}

// isAddonInstanceInstalled returns if the corresponding addon instance has installed=true status condition
func (r *olmReconciler) isAddonInstanceInstalled(ctx context.Context, addon *addonsv1alpha1.Addon) (bool, error) {
	addonInstance := &addonsv1alpha1.AddonInstance{}
	instanceKey := types.NamespacedName{
		Name:      addonsv1alpha1.DefaultAddonInstanceName,
		Namespace: GetCommonInstallOptions(addon).Namespace,
	}

	if err := r.client.Get(ctx, instanceKey, addonInstance); err != nil {
		return false, err
	}

	if installedCond := meta.FindStatusCondition(
		addonInstance.Status.Conditions,
		addonsv1alpha1.AddonInstanceConditionInstalled.String(),
	); installedCond == nil || installedCond.Status == v1.ConditionFalse {
		// Since the addon instance is not installed, we report the addon as unavailable.
		reportPendingAddonInstanceInstallation(addon)
		return false, nil
	}

	return meta.IsStatusConditionTrue(addonInstance.Status.Conditions, addonsv1alpha1.AddonInstanceConditionInstalled.String()), nil
}

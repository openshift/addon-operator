package addon

import (
	"context"
	"fmt"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func (r *olmReconciler) handleInstalledCondition(ctx context.Context,
	addon *addonsv1alpha1.Addon, addonCSVRef *operatorsv1.RichReference) (requeueResult, error) {
	// Handle missing CSV, addon might have been uninstalled?.
	if addonCSVRef == nil {
		return r.handleMissingCSV(ctx, addon)
	}
	csvPhase := getCSVPhase(addonCSVRef)
	// If csv is in the succeeded phase and the last observed CSV is empty, we know that
	// this CSV is coming up for the first time.
	if csvPhase == operatorsv1alpha1.CSVPhaseSucceeded && addon.Status.LastObservedAvailableCSV == "" {
		reportInstalledCondition(addon)
	}
	return resultNil, nil
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
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	_, found := deleteConfigMap.Labels[deleteConfigMapLabel]
	return found, nil
}

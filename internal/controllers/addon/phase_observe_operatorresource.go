package addon

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

// add mapping  if mapping changed requeue
// check status with current csv name

func (r *olmReconciler) observeOperatorResource(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
	csvKey client.ObjectKey,
) (requeueResult, error) {
	var commonInstallOptions addonsv1alpha1.AddonInstallOLMCommon
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMAllNamespaces:
		commonInstallOptions = addon.Spec.Install.
			OLMAllNamespaces.AddonInstallOLMCommon
	case addonsv1alpha1.OLMOwnNamespace:
		commonInstallOptions = addon.Spec.Install.
			OLMOwnNamespace.AddonInstallOLMCommon
	}
	operatorName := fmt.Sprintf("%s.%s", commonInstallOptions.PackageName, commonInstallOptions.Namespace)
	operatorKey := client.ObjectKey{
		Namespace: "",
		Name:      operatorName,
	}

	// add mapping
	changed := r.operatorResourceHandler.UpdateMap(addon, operatorKey)
	if changed {
		// Mapping changes need to requeue, because we could have lost events before or during
		// setting up the mapping
		return resultRetry, nil
	}

	operator := &operatorsv1.Operator{}

	if err := r.uncachedClient.Get(ctx, operatorKey, operator); err != nil {
		if k8serrors.IsNotFound(err) {
			return resultRetry, nil
		}
		return resultNil, fmt.Errorf("getting operator resource: %w", err)
	}

	var message string
	phase := csvSucceeded(csvKey, operator)

	// If the addon was being upgraded, we mark the upgrade as
	// concluded.
	if addonUpgradeConcluded(addon, csvKey, phase) {
		reportAddonUpgradeSucceeded(addon)
	}

	switch phase {
	case operatorsv1alpha1.CSVPhaseSucceeded:
		// do nothing here
	case operatorsv1alpha1.CSVPhaseFailed:
		message = "failed"
	default:
		message = "unkown/pending"
	}

	if message != "" {
		reportUnreadyCSV(addon, message)
		return resultRetry, nil
	}

	return resultNil, nil
}

func csvSucceeded(csv client.ObjectKey, operator *operatorsv1.Operator) operatorsv1alpha1.ClusterServiceVersionPhase {
	components := operator.Status.Components
	if components == nil {
		return ""
	}
	for _, component := range components.Refs {
		if component.Kind != "ClusterServiceVersion" {
			continue
		}
		if component.Name != csv.Name || component.Namespace != csv.Namespace {
			continue
		}
		compConditions := component.Conditions
		for _, c := range compConditions {
			if c.Type == "Succeeded" {
				if c.Status == "True" {
					return operatorsv1alpha1.CSVPhaseSucceeded
				} else {
					return operatorsv1alpha1.CSVPhaseFailed

				}
			}
		}
	}
	return ""
}

func addonUpgradeConcluded(addon *addonsv1alpha1.Addon, currentCSV client.ObjectKey, phase operatorsv1alpha1.ClusterServiceVersionPhase) bool {
	// Upgrading has concluded if a new CSV(compared to the last known available) has come up and is
	// in the succeeded phase.
	if addonUpgradeStarted(addon) {
		return currentCSV.String() != addon.Status.LastObservedAvailableCSV &&
			phase == operatorsv1alpha1.CSVPhaseSucceeded
	}
	return false
}

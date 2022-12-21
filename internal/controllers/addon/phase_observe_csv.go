package addon

import (
	"context"
	"fmt"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func (r *olmReconciler) observeCurrentCSV(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
	csvKey client.ObjectKey,
) (requeueResult, error) {
	csv := &operatorsv1alpha1.ClusterServiceVersion{}
	if err := r.uncachedClient.Get(ctx, csvKey, csv); err != nil {
		if k8serrors.IsNotFound(err) {
			return resultRetry, nil
		}

		return resultNil, fmt.Errorf("getting installed CSV: %w", err)
	}
	// If the addon was being upgraded, we mark the upgrade as
	// concluded.
	if addonUpgradeConcluded(addon, csv) {
		reportAddonUpgradeSucceeded(addon)
	}

	var message string
	switch csv.Status.Phase {
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

func addonUpgradeConcluded(addon *addonsv1alpha1.Addon, currentCSV *operatorsv1alpha1.ClusterServiceVersion) bool {
	// Upgrading has concluded if a new CSV(compared to the last known available) has come up and is
	// in the succeeded phase.
	if addonUpgradeStarted(addon) {
		return namespacedName(currentCSV) != addon.Status.LastObservedAvailableCSV &&
			currentCSV.Status.Phase == operatorsv1alpha1.CSVPhaseSucceeded
	}
	return false
}

package addon

import (
	"context"
	"fmt"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	commonInstallOptions := GetCommonInstallOptions(addon)
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

	addonCSVRef := findConcernedCSVReference(csvKey, operator)

	// Handle installed Condition.
	if res, err := r.handleInstalledCondition(ctx, addon, addonCSVRef); err != nil {
		return resultRetry, err
	} else if res != resultNil {
		// handleInstalledCondition asked us to stop further processing in this method,
		// so we return.
		return res, nil
	}

	// If CSV exists in the cluster, we go ahead check its phase.
	phase := getCSVPhase(addonCSVRef)

	// If the addon was being upgraded, we mark the upgrade as
	// concluded.
	if addonUpgradeConcluded(addon, csvKey, phase) {
		reportAddonUpgradeSucceeded(addon)
	}

	var message string
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

func getCSVPhase(csvReference *operatorsv1.RichReference) operatorsv1alpha1.ClusterServiceVersionPhase {
	if csvReference == nil {
		return ""
	}
	conditions := csvReference.Conditions
	for _, c := range conditions {
		if c.Type == "Succeeded" {
			if c.Status == "True" {
				return operatorsv1alpha1.CSVPhaseSucceeded
			} else {
				return operatorsv1alpha1.CSVPhaseFailed
			}
		}
	}
	return ""
}

func findConcernedCSVReference(csv client.ObjectKey, operator *operatorsv1.Operator) *operatorsv1.RichReference {
	components := operator.Status.Components
	if components == nil {
		return nil
	}
	for _, component := range components.Refs {
		if component.Kind == "ClusterServiceVersion" &&
			component.Name == csv.Name &&
			component.Namespace == csv.Namespace {
			return &component
		}
	}
	return nil
}

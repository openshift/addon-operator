package addon

import (
	"context"
	"fmt"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
)

// add mapping  if mapping changed requeue
// check status with current csv name

func (r *olmReconciler) observeOperatorResource(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
	csvKey client.ObjectKey,
) (requeueResult, error) {
	installOLMCommon, err := addon.GetInstallOLMCommon()
	if err != nil {
		return resultRetry, err
	}

	currentSub, err := r.GetSubscription(
		ctx,
		SubscriptionName(addon),
		installOLMCommon.Namespace,
	)
	if err != nil {
		return resultRetry, err
	}

	if currentSub.GetInstallPlanApproval() == operatorsv1alpha1.ApprovalManual {
		currentIp, err := r.GetInstallPlan(
			ctx,
			currentSub.Status.InstallPlanRef.Name,
			currentSub.Status.InstallPlanRef.Namespace,
		)
		if err != nil {
			return resultNil, err
		}

		if currentIp.Status.Phase == operatorsv1alpha1.InstallPlanPhaseRequiresApproval {
			reportInstallPlanPending(addon)
			// CSV will not be available at this stage
			return resultNil, nil
		}
	}

	operatorKey := client.ObjectKey{
		Namespace: "",
		Name:      generateOperatorResourceName(addon),
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
		message = "unknown/pending"
	}

	if message != "" {
		reportUnreadyCSV(addon, message)
		return resultRetry, nil
	}

	// If CSV is present and is in succeeded phase we report
	// the addon as available.
	if !r.IsPKOFailed(addon) {
		reportReadinessStatus(addon)
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

func generateOperatorResourceName(addon *addonsv1alpha1.Addon) string {
	commonInstallOptions := GetCommonInstallOptions(addon)
	return fmt.Sprintf("%s.%s", commonInstallOptions.PackageName, commonInstallOptions.Namespace)
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

func addonUpgradeConcluded(addon *addonsv1alpha1.Addon, currentCSV client.ObjectKey, phase operatorsv1alpha1.ClusterServiceVersionPhase) bool {
	// Upgrading has concluded if a new CSV(compared to the last known available) has come up and is
	// in the succeeded phase.
	if addonUpgradeStarted(addon) {
		return currentCSV.String() != addon.Status.LastObservedAvailableCSV &&
			phase == operatorsv1alpha1.CSVPhaseSucceeded
	}
	return false
}

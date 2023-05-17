package addon

import (
	"context"
	"time"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultDeleteTimeoutDuration = time.Hour * 1
	DELETION_RECONCILER_NAME     = "deletionReconciler"
)

type addonDeletionStrategy interface {
	NotifyAddon(context.Context, *addonsv1alpha1.Addon) error
	AckReceivedFromAddon(context.Context, *addonsv1alpha1.Addon) bool
}

type addonDeletionReconciler struct {
	strategies []addonDeletionStrategy
}

func (r *addonDeletionReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if !markedForDeletion(addon) || alreadyWaitingToBeDeleted(addon) {
		// Nothing to do.
		return ctrl.Result{}, nil
	}

	// if spec.DeleteAckRequired is false, we directly report ReadyToBeDeleted=true Status condition.
	if !addon.Spec.DeleteAckRequired {
		reportAddonReadyToBeDeletedStatus(addon, metav1.ConditionTrue)
		return ctrl.Result{}, nil
	}

	// We set ReadyToBeDeleted=false status condition in response to the delete signal received from OCM.
	reportAddonReadyToBeDeletedStatus(addon, metav1.ConditionFalse)

	if deletionTimedOut(addon) {
		reportAddonDeletionTimedOut(addon)
	}

	for _, strategy := range r.strategies {
		if err := strategy.NotifyAddon(ctx, addon); err != nil {
			return ctrl.Result{}, err
		}
		// If ack is received from the underlying addon, we report ReadyToBeDeleted = true.
		if strategy.AckReceivedFromAddon(ctx, addon) {
			removeDeleteTimeoutCondition(addon)
			reportAddonReadyToBeDeletedStatus(addon, metav1.ConditionTrue)
			return ctrl.Result{}, nil
		}
	}
	// If no ack is received from the addon, we arrange for a requeue after the deletetimeout duration.
	return ctrl.Result{RequeueAfter: deleteTimeoutInterval(addon)}, nil
}

// Deletion is timed out when the (ReadyToBeDeleted=false) condition's last transition time + deleteTimeoutInterval
// is after the current time.
func deletionTimedOut(addon *addonsv1alpha1.Addon) bool {
	readyToBeDeletedCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted)
	if readyToBeDeletedCond == nil || readyToBeDeletedCond.Status == metav1.ConditionTrue {
		return false
	}
	return readyToBeDeletedCond.LastTransitionTime.Add(deleteTimeoutInterval(addon)).After(time.Now())
}

func removeDeleteTimeoutCondition(addon *addonsv1alpha1.Addon) {
	meta.RemoveStatusCondition(&addon.Status.Conditions, addonsv1alpha1.DeleteTimeout)
}

func deleteTimeoutInterval(addon *addonsv1alpha1.Addon) time.Duration {
	OCMtimeoutDurationStr, found := addon.Annotations[addonsv1alpha1.DeleteTimeoutDuration]
	if !found {
		return defaultDeleteTimeoutDuration
	}

	if duration, err := time.ParseDuration(OCMtimeoutDurationStr); err != nil {
		return defaultDeleteTimeoutDuration
	} else {
		return duration
	}
}

// Addon is waiting to be deleted by OCM if ReadyToBeDeleted=true.
func alreadyWaitingToBeDeleted(addon *addonsv1alpha1.Addon) bool {
	cond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted)
	if cond == nil {
		return false
	}
	return cond.Status == metav1.ConditionTrue
}

func (r *addonDeletionReconciler) Name() string {
	return DELETION_RECONCILER_NAME
}

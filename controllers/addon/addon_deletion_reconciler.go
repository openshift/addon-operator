package addon

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"
)

const (
	defaultDeleteTimeoutDuration = time.Hour * 1
	DELETION_RECONCILER_NAME     = "deletionReconciler"
)

type addonDeletionHandler interface {
	NotifyAddon(context.Context, *addonsv1alpha1.Addon) error
	AckReceivedFromAddon(context.Context, *addonsv1alpha1.Addon) (bool, error)
}

type clock interface {
	Now() time.Time
}

type defaultClock struct{}

func (c defaultClock) Now() time.Time {
	return time.Now()
}

type addonDeletionReconciler struct {
	clock    clock
	handlers []addonDeletionHandler
	recorder *metrics.Recorder
}

func (r *addonDeletionReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	if !markedForDeletion(addon) || awaitingRemoteDeletion(addon) {
		// Nothing to do.
		return resultNil, nil
	}

	// if spec.DeleteAckRequired is false, we directly report ReadyToBeDeleted=true Status condition.
	if !addon.Spec.DeleteAckRequired {
		removeDeleteTimeoutCondition(addon)
		reportAddonReadyToBeDeletedStatus(addon, metav1.ConditionTrue)
		return resultNil, nil
	}

	// We set ReadyToBeDeleted=false status condition in response to the delete signal received from OCM.
	reportAddonReadyToBeDeletedStatus(addon, metav1.ConditionFalse)

	reconErr := metrics.NewReconcileError("addon", r.recorder, true)

	for _, handler := range r.handlers {
		if err := handler.NotifyAddon(ctx, addon); err != nil {
			err = reconErr.Join(err, controllers.ErrNotifyAddon)
			return resultNil, err
		}
		// If ack is received from the underlying addon, we report ReadyToBeDeleted = true.
		ackReceived, err := handler.AckReceivedFromAddon(ctx, addon)
		if err != nil {
			err = reconErr.Join(err, controllers.ErrAckReceivedFromAddon)
			return resultNil, err
		}
		if ackReceived {
			removeDeleteTimeoutCondition(addon)
			reportAddonReadyToBeDeletedStatus(addon, metav1.ConditionTrue)
			return resultNil, nil
		}
	}

	// If deletion has timed out.
	if r.deletionTimedOut(addon) {
		reportAddonDeletionTimedOut(addon)
		return resultNil, nil
	}
	// If no ack is received from the addon, we arrange for a requeue after the deletetimeout duration.
	return resultRequeueAfter(deleteTimeoutInterval(addon)), nil
}

// Deletion is timed out when (ReadyToBeDeleted=false) condition's last transition time + deleteTimeoutInterval
// is after the current time.
func (r *addonDeletionReconciler) deletionTimedOut(addon *addonsv1alpha1.Addon) bool {
	readyToBeDeletedCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted)
	if readyToBeDeletedCond == nil || readyToBeDeletedCond.Status == metav1.ConditionTrue {
		return false
	}
	return r.clock.Now().After(readyToBeDeletedCond.LastTransitionTime.Add(deleteTimeoutInterval(addon)))
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
func awaitingRemoteDeletion(addon *addonsv1alpha1.Addon) bool {
	cond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted)
	if cond == nil {
		return false
	}
	return cond.Status == metav1.ConditionTrue
}

func (r *addonDeletionReconciler) Name() string {
	return DELETION_RECONCILER_NAME
}

func (r *addonDeletionReconciler) Order() subReconcilerOrder {
	return AddonDeletionReconcilerOrder
}

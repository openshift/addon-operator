package addon

import (
	"context"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The addon instance deletion strategy uses the addon instance to coordinate addon deletion.
// To notify the addon of the deletion process, we first set addoninstance.Spec.MarkedForDeletion=true.
// The addon's controller reconciles this change and cleans up its resources.
// Once done cleaning up, the addon's controller would set
// AddonInstanceConditionReadyToBeDeleted=true status condition to the addoninstance.
type addonInstanceDeletionStrategy struct {
	client client.Client
}

func (a *addonInstanceDeletionStrategy) NotifyAddon(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	currentAddonInstance := &addonsv1alpha1.AddonInstance{}
	addonNS := GetCommonInstallOptions(addon).Namespace
	if err := a.fetchAddonInstance(ctx, addonNS, currentAddonInstance); err != nil {
		if errors.IsNotFound(err) {
			// We return without errors on notfound errors, as the addon instance obj would get created
			// eventually by the subsequent sub-reconcilers and we would be requeued on that event.
			return nil
		}
		return err
	}
	if !currentAddonInstance.Spec.MarkedForDeletion {
		currentAddonInstance.Spec.MarkedForDeletion = true
		return a.client.Update(ctx, currentAddonInstance)
	}
	return nil
}

func (a *addonInstanceDeletionStrategy) AckReceivedFromAddon(
	ctx context.Context,
	addon *addonsv1alpha1.Addon) (bool, error) {
	currentAddonInstance := &addonsv1alpha1.AddonInstance{}
	addonNS := GetCommonInstallOptions(addon).Namespace
	if err := a.fetchAddonInstance(ctx, addonNS, currentAddonInstance); err != nil {
		// We return without errors on notfound errors, as the addon instance obj would get created
		// eventually by the subsequent sub-reconcilers and we would be requeued on that event.
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return hasReadyToBeDeletedStatusCondition(currentAddonInstance, metav1.ConditionTrue), nil
}

func (a *addonInstanceDeletionStrategy) fetchAddonInstance(
	ctx context.Context, addonNS string, instance *addonsv1alpha1.AddonInstance) error {
	addonInstanceKey := types.NamespacedName{
		Name:      addonsv1alpha1.DefaultAddonInstanceName,
		Namespace: addonNS,
	}
	return a.client.Get(ctx, addonInstanceKey, instance)
}

func hasReadyToBeDeletedStatusCondition(
	instance *addonsv1alpha1.AddonInstance, expectedValue metav1.ConditionStatus) bool {
	requiredCond := meta.FindStatusCondition(
		instance.Status.Conditions,
		string(addonsv1alpha1.AddonInstanceConditionReadyToBeDeleted),
	)
	if requiredCond == nil {
		return false
	}
	return requiredCond.Status == expectedValue
}

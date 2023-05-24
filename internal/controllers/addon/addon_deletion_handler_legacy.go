package addon

import (
	"context"
	"fmt"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

var DeleteConfigMapLabel = "api.openshift.com/addon-%v-delete"

// Legacy deletion strategy creates a config map with specific label to notify the addon
// of the deletion process. The addon notices this CM and cleans up its resources and
// then deletes its CSV. We take this CSV going missing as an ack from the underlying
// addon.
type legacyDeletionHandler struct {
	client         client.Client
	uncachedClient client.Client
}

func (l *legacyDeletionHandler) NotifyAddon(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	currentDeleteCM := &corev1.ConfigMap{}
	addonTargetNS := GetCommonInstallOptions(addon).Namespace

	if err := l.uncachedClient.Get(ctx, types.NamespacedName{Name: addon.Name, Namespace: addonTargetNS}, currentDeleteCM); err != nil {
		if errors.IsNotFound(err) {
			return l.createDeleteConfigMap(ctx, addon)
		}
		return err
	}

	if _, labelFound := currentDeleteCM.Labels[fmt.Sprintf(DeleteConfigMapLabel, addon.Name)]; labelFound {
		return nil
	}

	modifiedCM := currentDeleteCM.DeepCopy()

	// If delete label is missing, we add it to the object and patch it.
	if modifiedCM.Labels == nil {
		modifiedCM.Labels = make(map[string]string)
	}
	modifiedCM.Labels[fmt.Sprintf(DeleteConfigMapLabel, addon.Name)] = ""
	return l.client.Patch(ctx, modifiedCM, client.MergeFrom(currentDeleteCM))
}

func (l *legacyDeletionHandler) AckReceivedFromAddon(ctx context.Context, addon *addonsv1alpha1.Addon) (bool, error) {
	operatorKey := client.ObjectKey{
		Namespace: "",
		Name:      generateOperatorResourceName(addon),
	}

	operator := &operatorsv1.Operator{}

	if err := l.uncachedClient.Get(ctx, operatorKey, operator); err != nil {
		// We return without errors on notfound errors, as the obj would get created
		// eventually and we would be requeued on that event.
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Fetch current CSV key from subscription.
	currentSubscription := &operatorsv1alpha1.Subscription{}
	addonTargetNS := GetCommonInstallOptions(addon).Namespace
	err := l.client.Get(ctx, client.ObjectKey{
		Name:      SubscriptionName(addon),
		Namespace: addonTargetNS,
	}, currentSubscription)
	if err != nil {
		// We return without errors on notfound errors, as the obj would get created
		// eventually by the subsequent sub-reconcilers and we would be requeued on that event.
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	// The addon deletes its CSV after it cleans up its external resources.
	csvKey := types.NamespacedName{Namespace: addonTargetNS, Name: currentSubscription.Status.CurrentCSV}
	return CSVmissing(operator, csvKey), nil
}

func CSVmissing(operator *operatorsv1.Operator, csvKey types.NamespacedName) bool {
	return findConcernedCSVReference(csvKey, operator) == nil
}

func (l *legacyDeletionHandler) createDeleteConfigMap(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	addonTargetNS := GetCommonInstallOptions(addon).Namespace

	desiredCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name,
			Namespace: addonTargetNS,
			Labels: map[string]string{
				fmt.Sprintf(DeleteConfigMapLabel, addon.Name): "",
			},
		},
	}
	return l.client.Create(ctx, desiredCM)
}

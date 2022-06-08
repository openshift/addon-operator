package addon

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

func (r *olmReconciler) ensureSubscription(
	ctx context.Context,
	log logr.Logger,
	addon *addonsv1alpha1.Addon,
	catalogSource *operatorsv1alpha1.CatalogSource,
) (
	requeueResult,
	client.ObjectKey,
	error,
) {
	var commonInstallOptions addonsv1alpha1.AddonInstallOLMCommon
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMAllNamespaces:
		commonInstallOptions = addon.Spec.Install.
			OLMAllNamespaces.AddonInstallOLMCommon
	case addonsv1alpha1.OLMOwnNamespace:
		commonInstallOptions = addon.Spec.Install.
			OLMOwnNamespace.AddonInstallOLMCommon
	}
	subscriptionConfigObject := createSubscriptionConfigObject(commonInstallOptions)
	desiredSubscription := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name,
			Namespace: commonInstallOptions.Namespace,
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			CatalogSource:          catalogSource.Name,
			CatalogSourceNamespace: catalogSource.Namespace,
			Channel:                commonInstallOptions.Channel,
			Package:                commonInstallOptions.PackageName,
			Config:                 subscriptionConfigObject,
			// InstallPlanApproval is deliberately unmanaged
			// API default is `Automatic`
			// Legacy behavior of existing managed-tenants tooling is:
			// All addons initially have to be installed with `Automatic`
			// so that the very first InstallPlan succeedes
			// but some addons want to take control of upgrades and thus
			// change the Subscription.Spec.InstallPlanApproval value to `Manual`
			// ATTENTION: When reconciling the subscription, we need to
			// make sure to keep the current value of this field
		},
	}
	controllers.AddCommonLabels(desiredSubscription, addon)
	if err := controllerutil.SetControllerReference(addon, desiredSubscription, r.scheme); err != nil {
		return resultNil, client.ObjectKey{}, fmt.Errorf("setting controller reference: %w", err)
	}

	observedSubscription, err := r.reconcileSubscription(
		ctx, desiredSubscription, addon.Spec.ResourceAdoptionStrategy)
	if err != nil {
		return resultNil, client.ObjectKey{}, fmt.Errorf("reconciling Subscription: %w", err)
	}

	if len(observedSubscription.Status.InstalledCSV) == 0 ||
		len(observedSubscription.Status.CurrentCSV) == 0 {
		log.Info("requeue", "reason", "csv not linked in subscription")
		return resultRetry, client.ObjectKey{}, nil
	}

	installedCSVKey := client.ObjectKey{
		Name:      observedSubscription.Status.InstalledCSV,
		Namespace: commonInstallOptions.Namespace,
	}
	currentCSVKey := client.ObjectKey{
		Name:      observedSubscription.Status.CurrentCSV,
		Namespace: commonInstallOptions.Namespace,
	}

	changed := r.csvEventHandler.ReplaceMap(addon, installedCSVKey, currentCSVKey)
	if changed {
		// Mapping changes need to requeue, because we could have lost events before or during
		// setting up the mapping, see csvEventHandler implementation for a longer description.
		log.Info("requeue", "reason", "csv-addon mapping changed")
		return resultRetry, client.ObjectKey{}, nil
	}

	return resultNil, currentCSVKey, nil
}

func (r *olmReconciler) reconcileSubscription(
	ctx context.Context,
	subscription *operatorsv1alpha1.Subscription,
	strategy addonsv1alpha1.ResourceAdoptionStrategyType,
) (currentSubscription *operatorsv1alpha1.Subscription, err error) {
	currentSubscription = &operatorsv1alpha1.Subscription{}
	err = r.client.Get(ctx, client.ObjectKey{
		Name:      subscription.Name,
		Namespace: subscription.Namespace,
	}, currentSubscription)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return subscription, r.client.Create(ctx, subscription)
		}
		return nil, err
	}

	// keep installPlanApproval value of existing object
	subscription.Spec.InstallPlanApproval = currentSubscription.Spec.InstallPlanApproval

	// TODO: remove after the syncSetMigration is finish and the resourceAdoptionStrategy is discontinued (after this comment)
	ownedByAddon := controllers.HasEqualControllerReference(currentSubscription, subscription)
	if strategy != addonsv1alpha1.ResourceAdoptionAdoptAll && !ownedByAddon {
		return nil, controllers.ErrNotOwnedByUs
	} else if strategy == addonsv1alpha1.ResourceAdoptionAdoptAll && !ownedByAddon {
		currentSubscription.OwnerReferences = subscription.OwnerReferences
	}
	// TODO: remove after the syncSetMigration is finish and the resourceAdoptionStrategy is discontinued (before this comment)

	// only update when spec or labels has changed
	currentLabels := currentSubscription.Labels
	newLabels := labels.Merge(currentLabels, subscription.Labels)
	specChanged := !equality.Semantic.DeepEqual(subscription.Spec, currentSubscription.Spec)
	// TODO: remove just `!ownedByAddon` after the syncSetMigration is finish and the resourceAdoptionStrategy is discontinued
	if specChanged || !ownedByAddon || !labels.Equals(currentLabels, newLabels) {
		// copy new spec into existing object and update in the k8s api
		currentSubscription.Spec = subscription.Spec
		currentSubscription.Labels = newLabels
		return currentSubscription, r.client.Update(ctx, currentSubscription)
	}
	return currentSubscription, nil
}

// Returns the subscription config object to be created from the passed AddonInstallOLMCommon object
func createSubscriptionConfigObject(commonInstallOptions addonsv1alpha1.AddonInstallOLMCommon) *operatorsv1alpha1.SubscriptionConfig {
	if commonInstallOptions.Config != nil {
		subscriptionConfig := &operatorsv1alpha1.SubscriptionConfig{
			Env: getSubscriptionEnvObjects(commonInstallOptions.Config.EnvironmentVariables),
		}
		return subscriptionConfig
	}
	return nil
}

// Converts addonsv1alpha1.EnvObjects to corev1.EnvVar's
func getSubscriptionEnvObjects(envObjects []addonsv1alpha1.EnvObject) []corev1.EnvVar {
	subscriptionEnvObjects := []corev1.EnvVar{}
	for _, envObject := range envObjects {
		currentEnvObj := corev1.EnvVar{
			Name:  envObject.Name,
			Value: envObject.Value,
		}
		subscriptionEnvObjects = append(subscriptionEnvObjects, currentEnvObj)
	}
	return subscriptionEnvObjects
}

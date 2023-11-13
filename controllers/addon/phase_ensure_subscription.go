package addon

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
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
	commonInstallOptions, err := addon.GetInstallOLMCommon()
	if err != nil {
		return resultNil, client.ObjectKey{}, err
	}

	subscriptionConfigObject := createSubscriptionConfigObject(commonInstallOptions)
	desiredSubscription := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SubscriptionName(addon),
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
			// so that the very first InstallPlan succeeds
			// but some addons want to take control of upgrades and thus
			// change the Subscription.Spec.InstallPlanApproval value to `Manual`
			// ATTENTION: When reconciling the subscription, we need to
			// make sure to keep the current value of this field
		},
	}
	controllers.AddCommonLabels(desiredSubscription, addon)
	controllers.AddCommonAnnotations(desiredSubscription, addon)
	if err := controllerutil.SetControllerReference(addon, desiredSubscription, r.scheme); err != nil {
		return resultNil, client.ObjectKey{}, fmt.Errorf("setting controller reference: %w", err)
	}

	observedSubscription, err := r.reconcileSubscription(ctx, desiredSubscription)
	if err != nil {
		return resultNil, client.ObjectKey{}, fmt.Errorf("reconciling Subscription: %w", err)
	}

	if len(observedSubscription.Status.InstalledCSV) == 0 ||
		len(observedSubscription.Status.CurrentCSV) == 0 {
		// This case seems to happen when e.g. dependency declarations in the bundle are missing.
		//
		// Example Subscription Condition:
		// message: 'constraints not satisfiable: subscription addon-mcg-osd-dev requires addon-mcg-osd-dev-catalog/redhat-data-federation/alpha/mcg-osd-deployer.v1.0.0, subscription addon-mcg-osd-dev exists, bundle mcg-osd-deployer.v1.0.0 requires an operator with package: mcg-operator and with version in range: 4.11.0'
		// reason: ConstraintsNotSatisfiable
		// status: "True"
		// type: ResolutionFailed
		meta.SetStatusCondition(&addon.Status.Conditions, metav1.Condition{
			Type:               addonsv1alpha1.Available,
			Status:             metav1.ConditionFalse,
			Reason:             addonsv1alpha1.AddonReasonUnreadyCSV,
			Message:            "CSV not linked in Subscription. Dependency issue?",
			ObservedGeneration: addon.Generation,
		})
		addon.Status.ObservedGeneration = addon.Generation
		addon.Status.Phase = addonsv1alpha1.PhaseError

		log.Info("requeue", "reason", "csv not linked in subscription")
		return resultRetry, client.ObjectKey{}, nil
	}

	currentCSVKey := client.ObjectKey{
		Name:      observedSubscription.Status.CurrentCSV,
		Namespace: commonInstallOptions.Namespace,
	}

	return resultNil, currentCSVKey, nil
}

func (r *olmReconciler) reconcileSubscription(
	ctx context.Context,
	subscription *operatorsv1alpha1.Subscription,
) (currentSubscription *operatorsv1alpha1.Subscription, err error) {
	currentSubscription, err = r.GetSubscription(
		ctx,
		subscription.Name,
		subscription.Namespace,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return subscription, r.client.Create(ctx, subscription)
		}
		return nil, err
	}

	// keep installPlanApproval value of existing object
	subscription.Spec.InstallPlanApproval = currentSubscription.Spec.InstallPlanApproval

	// Only update when spec, controllerRef, or labels have changed
	specChanged := !equality.Semantic.DeepEqual(subscription.Spec, currentSubscription.Spec)
	ownedByAddon := controllers.HasSameController(currentSubscription, subscription)
	currentLabels := labels.Set(currentSubscription.Labels)
	newLabels := labels.Merge(currentLabels, labels.Set(subscription.Labels))
	if specChanged || !ownedByAddon || !labels.Equals(currentLabels, newLabels) {
		currentSubscription.Spec = subscription.Spec
		currentSubscription.OwnerReferences = subscription.OwnerReferences
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

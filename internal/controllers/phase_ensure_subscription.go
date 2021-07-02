package controllers

import (
	"context"
	"fmt"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func (r *AddonReconciler) ensureSubscription(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
	catalogSource *operatorsv1alpha1.CatalogSource,
) (*operatorsv1alpha1.Subscription, error) {
	var commonInstallOptions addonsv1alpha1.AddonInstallCommon
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.AllNamespaces:
		commonInstallOptions = addon.Spec.Install.
			AllNamespaces.AddonInstallCommon
	case addonsv1alpha1.OwnNamespace:
		commonInstallOptions = addon.Spec.Install.
			OwnNamespace.AddonInstallCommon
	}

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
			InstallPlanApproval:    operatorsv1alpha1.ApprovalAutomatic,
		},
	}
	addCommonLabels(desiredSubscription.Labels, addon)
	if err := controllerutil.SetControllerReference(addon, desiredSubscription, r.Scheme); err != nil {
		return nil, fmt.Errorf("setting controller reference: %w", err)
	}

	observedSubscription, err := r.reconcileSubscription(
		ctx, desiredSubscription)
	if err != nil {
		return nil, fmt.Errorf("reconciling Subscription: %w", err)
	}

	installedCSVKey := client.ObjectKey{
		Name:      observedSubscription.Status.InstalledCSV,
		Namespace: commonInstallOptions.Namespace,
	}
	currentCSVKey := client.ObjectKey{
		Name:      observedSubscription.Status.CurrentCSV,
		Namespace: commonInstallOptions.Namespace,
	}

	// Check status via CSVs
	installedCSV := &operatorsv1alpha1.ClusterServiceVersion{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      observedSubscription.Status.InstalledCSV,
		Namespace: commonInstallOptions.Namespace,
	}, installedCSV); err != nil {
		return nil, fmt.Errorf("getting installed CSV: %w", err)
	}

	changed := r.csvEventHandler.ReplaceMap(
		addon, installedCSVKey, currentCSVKey)
	if changed {
		// MUST REQUEUE ONCE
	}

	return observedSubscription, nil
}

func (r *AddonReconciler) reconcileSubscription(
	ctx context.Context,
	subscription *operatorsv1alpha1.Subscription,
) (currentSubscription *operatorsv1alpha1.Subscription, err error) {
	currentSubscription = &operatorsv1alpha1.Subscription{}
	err = r.Get(ctx, client.ObjectKey{
		Name:      subscription.Name,
		Namespace: subscription.Namespace,
	}, currentSubscription)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return subscription, r.Create(ctx, subscription)
		}
		return nil, err
	}
	// only update when spec has changed
	if !equality.Semantic.DeepEqual(
		subscription.Spec, currentSubscription.Spec) {
		// copy new spec into existing object and update in the k8s api
		currentSubscription.Spec = subscription.Spec
		return currentSubscription, r.Update(ctx, currentSubscription)
	}
	return currentSubscription, nil
}

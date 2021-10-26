package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

const catalogSourcePublisher = "OSD Red Hat Addons"

type ensureCatalogSourceResult int

const (
	ensureCatalogSourceResultNil   ensureCatalogSourceResult = iota
	ensureCatalogSourceResultStop  ensureCatalogSourceResult = iota
	ensureCatalogSourceResultRetry ensureCatalogSourceResult = iota
)

// Ensure existence of the CatalogSource specified in the given Addon resource
// returns an ensureCatalogSourceResult that signals the caller if they have to
// stop or retry reconciliation of the surrounding Addon resource
func (r *AddonReconciler) ensureCatalogSource(
	ctx context.Context, log logr.Logger, addon *addonsv1alpha1.Addon,
) (ensureCatalogSourceResult, *operatorsv1alpha1.CatalogSource, error) {
	targetNamespace, catalogSourceImage, stop, err := r.parseAddonInstallConfig(ctx, log, addon)
	if err != nil {
		return ensureCatalogSourceResultNil, nil, err
	}
	if stop {
		return ensureCatalogSourceResultStop, nil, nil
	}

	catalogSource := &operatorsv1alpha1.CatalogSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name,
			Namespace: targetNamespace,
		},
		Spec: operatorsv1alpha1.CatalogSourceSpec{
			SourceType:  operatorsv1alpha1.SourceTypeGrpc,
			Publisher:   catalogSourcePublisher,
			DisplayName: addon.Spec.DisplayName,
			Image:       catalogSourceImage,
		},
	}

	addCommonLabels(catalogSource.Labels, addon)

	if err := controllerutil.SetControllerReference(addon, catalogSource, r.Scheme); err != nil {
		return ensureCatalogSourceResultNil, nil, err
	}

	var observedCatalogSource *operatorsv1alpha1.CatalogSource
	{
		var err error
		observedCatalogSource, err = reconcileCatalogSource(ctx, r.Client, catalogSource)
		if err != nil {
			return ensureCatalogSourceResultNil, nil, err
		}
	}

	if observedCatalogSource.Status.GRPCConnectionState == nil {
		if err := r.reportPendingStatus(
			ctx,
			addon,
			addonsv1alpha1.AddonReasonUnreadyCatalogSource,
			"CatalogSource connection is not ready: .Status.GRPConnectionState is nil",
		); err != nil {
			return ensureCatalogSourceResultNil, nil, err
		}
		return ensureCatalogSourceResultRetry, nil, nil
	}
	if observedCatalogSource.Status.GRPCConnectionState.LastObservedState != "READY" {
		msg := fmt.Sprintf(
			"CatalogSource connection is not ready: .Status.GRPCConnectionState.LastObservedState == %s",
			observedCatalogSource.Status.GRPCConnectionState.LastObservedState,
		)
		if err := r.reportPendingStatus(ctx, addon, addonsv1alpha1.AddonReasonUnreadyCatalogSource, msg); err != nil {
			return ensureCatalogSourceResultNil, nil, err
		}
		return ensureCatalogSourceResultRetry, nil, err
	}

	return ensureCatalogSourceResultNil, observedCatalogSource, nil
}

// reconciles a CatalogSource and returns a new CatalogSource object with observed state.
// Warning: Will adopt existing CatalogSource
func reconcileCatalogSource(ctx context.Context, c client.Client, catalogSource *operatorsv1alpha1.CatalogSource) (
	*operatorsv1alpha1.CatalogSource, error) {
	currentCatalogSource := &operatorsv1alpha1.CatalogSource{}

	{
		err := c.Get(ctx, client.ObjectKey{
			Name:      catalogSource.Name,
			Namespace: catalogSource.Namespace,
		}, currentCatalogSource)
		if err != nil {
			if k8sApiErrors.IsNotFound(err) {
				return catalogSource, c.Create(ctx, catalogSource)
			}
			return nil, err
		}
	}

	// only update when spec has changed
	if !equality.Semantic.DeepEqual(catalogSource.Spec, currentCatalogSource.Spec) {
		// copy new spec into existing object and update in the k8s api
		currentCatalogSource.Spec = catalogSource.Spec
		return currentCatalogSource, c.Update(ctx, currentCatalogSource)
	}

	return currentCatalogSource, nil
}

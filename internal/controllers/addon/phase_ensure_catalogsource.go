package addon

import (
	"context"
	"fmt"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

const catalogSourcePublisher = "OSD Red Hat Addons"

// Ensure existence of the CatalogSource specified in the given Addon resource
// returns an ensureCatalogSourceResult that signals the caller if they have to
// stop or retry reconciliation of the surrounding Addon resource
func (r *olmReconciler) ensureCatalogSource(
	ctx context.Context, addon *addonsv1alpha1.Addon,
) (requeueResult, *operatorsv1alpha1.CatalogSource, error) {
	log := controllers.LoggerFromContext(ctx)

	commonConfig, stop := parseAddonInstallConfig(log, addon)
	if stop {
		return resultStop, nil, nil
	}

	catalogSource := &operatorsv1alpha1.CatalogSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CatalogSourceName(addon),
			Namespace: commonConfig.Namespace,
		},
		Spec: operatorsv1alpha1.CatalogSourceSpec{
			SourceType:    operatorsv1alpha1.SourceTypeGrpc,
			Publisher:     catalogSourcePublisher,
			DisplayName:   addon.Spec.DisplayName,
			Image:         commonConfig.CatalogSourceImage,
			GrpcPodConfig: &operatorsv1alpha1.GrpcPodConfig{SecurityContextConfig: operatorsv1alpha1.Restricted},
		},
	}
	if len(commonConfig.PullSecretName) > 0 {
		catalogSource.Spec.Secrets = []string{
			commonConfig.PullSecretName,
		}
	}

	controllers.AddCommonLabels(catalogSource, addon)
	controllers.AddCommonAnnotations(catalogSource, addon)

	if err := controllerutil.SetControllerReference(addon, catalogSource, r.scheme); err != nil {
		return resultNil, nil, err
	}

	var observedCatalogSource *operatorsv1alpha1.CatalogSource
	{
		var err error
		observedCatalogSource, err = reconcileCatalogSource(ctx, r.client, catalogSource)
		if err != nil {
			return resultNil, nil, err
		}
	}

	if observedCatalogSource.Status.GRPCConnectionState == nil {
		reportCatalogSourceUnreadinessStatus(addon, ".Status.GRPCConnectionState is nil")
		return resultRetry, nil, nil
	}
	if observedCatalogSource.Status.GRPCConnectionState.LastObservedState != "READY" {
		reportCatalogSourceUnreadinessStatus(
			addon,
			fmt.Sprintf(
				".Status.GRPCConnectionState.LastObservedState == %s",
				observedCatalogSource.Status.GRPCConnectionState.LastObservedState,
			),
		)
		return resultRetry, nil, nil
	}

	return resultNil, observedCatalogSource, nil
}

func (r *olmReconciler) ensureAdditionalCatalogSources(
	ctx context.Context, addon *addonsv1alpha1.Addon,
) (requeueResult, error) {
	if !HasAdditionalCatalogSources(addon) {
		return resultNil, nil
	}
	additionalCatalogSrcs, targetNamespace, pullSecret, stop := parseAddonInstallConfigForAdditionalCatalogSources(
		controllers.LoggerFromContext(ctx),
		addon,
	)
	if stop {
		return resultStop, nil
	}
	for _, additionalCatalogSrc := range additionalCatalogSrcs {
		currentCatalogSrc := &operatorsv1alpha1.CatalogSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      additionalCatalogSrc.Name,
				Namespace: targetNamespace,
			},
			Spec: operatorsv1alpha1.CatalogSourceSpec{
				SourceType:    operatorsv1alpha1.SourceTypeGrpc,
				Publisher:     catalogSourcePublisher,
				DisplayName:   addon.Spec.DisplayName,
				Image:         additionalCatalogSrc.Image,
				GrpcPodConfig: &operatorsv1alpha1.GrpcPodConfig{SecurityContextConfig: operatorsv1alpha1.Restricted},
			},
		}

		if len(pullSecret) > 0 {
			currentCatalogSrc.Spec.Secrets = []string{
				pullSecret,
			}
		}

		controllers.AddCommonLabels(currentCatalogSrc, addon)
		controllers.AddCommonAnnotations(currentCatalogSrc, addon)

		if err := controllerutil.SetControllerReference(addon, currentCatalogSrc, r.scheme); err != nil {
			return resultNil, err
		}
		var observedCatalogSource *operatorsv1alpha1.CatalogSource
		var err error
		observedCatalogSource, err = reconcileCatalogSource(ctx, r.client, currentCatalogSrc)
		if err != nil {
			return resultNil, err
		}

		if observedCatalogSource.Status.GRPCConnectionState == nil {
			reportAdditionalCatalogSourceUnreadinessStatus(addon, ".Status.GRPCConnectionState is nil")
			return resultRetry, nil
		}
		if observedCatalogSource.Status.GRPCConnectionState.LastObservedState != "READY" {
			reportAdditionalCatalogSourceUnreadinessStatus(
				addon,
				fmt.Sprintf(
					".Status.GRPCConnectionState.LastObservedState == %s",
					observedCatalogSource.Status.GRPCConnectionState.LastObservedState,
				),
			)
			return resultRetry, nil
		}
	}
	return resultNil, nil
}

// reconciles a CatalogSource and returns a new CatalogSource object with updated state.
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

	ownedByAddon := controllers.HasSameController(currentCatalogSource, catalogSource)
	specChanged := !equality.Semantic.DeepEqual(catalogSource.Spec, currentCatalogSource.Spec)
	currentLabels := labels.Set(currentCatalogSource.Labels)
	newLabels := labels.Merge(currentLabels, labels.Set(catalogSource.Labels))
	if specChanged || !ownedByAddon || !labels.Equals(newLabels, currentLabels) {
		currentCatalogSource.Spec = catalogSource.Spec
		currentCatalogSource.OwnerReferences = catalogSource.OwnerReferences
		currentCatalogSource.Labels = newLabels
		return currentCatalogSource, c.Update(ctx, currentCatalogSource)
	}

	return currentCatalogSource, nil
}

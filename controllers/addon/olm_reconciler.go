package addon

import (
	"context"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"
)

const OLM_RECONCILER_NAME = "olmReconciler"

type olmReconciler struct {
	scheme                  *runtime.Scheme
	client                  client.Client
	uncachedClient          client.Client
	operatorResourceHandler operatorResourceHandler
	recorder                *metrics.Recorder
}

func (r *olmReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	log := controllers.LoggerFromContext(ctx)

	var err error
	reconErr := metrics.NewReconcileError("addon", r.recorder, true)

	// Phase 1.
	// Ensure OperatorGroup
	if requeueResult, err := r.ensureOperatorGroup(ctx, addon); err != nil {
		err = reconErr.Join(err, controllers.ErrEnsureOperatorGroup)
		return resultNil, err
	} else if !requeueResult.IsZero() {
		return requeueResult, nil
	}

	// Phase 2.
	// Ensure NetworkPolicy for CatalogSources
	// Note: This Phase must preempt CatalogSource reconciliation
	// as the CatalogSources will never report 'ready' if OLM
	// cannot verify the status of the GRPC connection.
	if requeueResult, err := r.ensureCatalogSourcesNetworkPolicy(ctx, addon); err != nil {
		err = reconErr.Join(err, controllers.ErrEnsureNetworkPolicy)
		return resultNil, err
	} else if !requeueResult.IsZero() {
		return requeueResult, nil
	}

	// Phase 3.
	// Ensure CatalogSource
	var (
		catalogSource *operatorsv1alpha1.CatalogSource
		result        subReconcilerResult
	)
	if result, catalogSource, err = r.ensureCatalogSource(ctx, addon); err != nil {
		err = reconErr.Join(err, controllers.ErrEnsureCatalogSource)
		return resultNil, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Phase 4.
	// Ensure Additional CatalogSources
	if result, err = r.ensureAdditionalCatalogSources(ctx, addon); err != nil {
		err = reconErr.Join(err, controllers.ErrEnsureAdditionalCatalogSource)
		return resultNil, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Phase 5.
	// Ensure Subscription for this Addon.
	result, currentCSVKey, err := r.ensureSubscription(
		ctx, log.WithName("phase-ensure-subscription"),
		addon, catalogSource)
	if err != nil {
		err = reconErr.Join(err, controllers.ErrReconcileSubscription)
		return resultNil, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Phase 6
	// Observe operator API
	if result, err := r.observeOperatorResource(ctx, addon, currentCSVKey); err != nil {
		err = reconErr.Join(err, controllers.ErrObserveCSV)
		return resultNil, err
	} else if !result.IsZero() {
		return result, nil
	}
	reportLastObservedAvailableCSV(addon, currentCSVKey.String())
	return resultNil, nil
}

func (r *olmReconciler) Name() string {
	return OLM_RECONCILER_NAME
}

func (r *olmReconciler) Order() subReconcilerOrder {
	return OLMReconcilerOrder
}

// Gets a subscription by ObjectKey
func (r *olmReconciler) GetSubscription(
	ctx context.Context,
	name string,
	namespace string,
) (*operatorsv1alpha1.Subscription, error) {
	destSub := operatorsv1alpha1.Subscription{}
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}

	if err := r.client.Get(ctx, key, &destSub); err != nil {
		return nil, err
	}
	return &destSub, nil
}

// Gets an InstallPlan by ObjectKey
func (r *olmReconciler) GetInstallPlan(
	ctx context.Context,
	name string,
	namespace string,
) (*operatorsv1alpha1.InstallPlan, error) {
	destIp := operatorsv1alpha1.InstallPlan{}
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}
	if err := r.client.Get(ctx, key, &destIp); err != nil {
		return nil, err
	}
	return &destIp, nil
}

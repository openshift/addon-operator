package addon

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

type olmReconciler struct {
	scheme          *runtime.Scheme
	client          client.Client
	log             logr.Logger
	csvEventHandler csvEventHandler
}

func (r *olmReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (ctrl.Result, error) {

	// get addon's namespaced name from context and inject into logger
	log := r.log.WithValues("addon", ctx.Value(addonNamespaceNameKey))

	var err error

	// Phase 1.
	// Ensure OperatorGroup
	if requeueResult, err := r.ensureOperatorGroup(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure OperatorGroup: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// Phase 2.
	// Ensure CatalogSource
	var (
		catalogSource *operatorsv1alpha1.CatalogSource
		requeueResult requeueResult
	)
	if requeueResult, catalogSource, err = r.ensureCatalogSource(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure CatalogSource: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// Phase 3.
	// Ensure Additional CatalogSources
	if requeueResult, err = r.ensureAdditionalCatalogSources(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure additional CatalogSource: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// Phase 4.
	// Ensure Subscription for this Addon.
	requeueResult, currentCSVKey, err := r.ensureSubscription(
		ctx, log.WithName("phase-ensure-subscription"),
		addon, catalogSource)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure Subscription: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// Phase 5.
	// Observe current csv
	if requeueResult, err := r.observeCurrentCSV(ctx, addon, currentCSVKey); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to observe current CSV: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}
	return reconcile.Result{}, nil
}

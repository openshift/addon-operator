package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

// Default timeout when we do a manual RequeueAfter
const defaultRetryAfterTime = 10 * time.Second

type AddonReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *AddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.Addon{}).
		Owns(&corev1.Namespace{}).
		Owns(&operatorsv1.OperatorGroup{}).
		Owns(&operatorsv1alpha1.CatalogSource{}).
		Complete(r)
}

// AddonReconciler/Controller entrypoint
func (r *AddonReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("addon", req.NamespacedName.String())

	addon := &addonsv1alpha1.Addon{}
	err := r.Get(ctx, req.NamespacedName, addon)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch Addon")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !addon.DeletionTimestamp.IsZero() {
		// Addon was already deleted and we don't need to do any cleanup for now
		// since kubernetes will garbage collect our child objects

		if addon.Status.Phase == addonsv1alpha1.PhaseTerminating {
			return ctrl.Result{}, nil
		}

		err := r.reportTerminationStatus(ctx, addon)
		if err != nil {
			log.Error(err, "failed to update termination status")
		}
		return ctrl.Result{}, err
	}

	// Phase 1.
	// Ensure wanted namespaces
	{
		stopAndRetry, err := r.ensureWantedNamespaces(ctx, addon)
		if err != nil {
			log.Error(err, "failed to ensure wanted namespaces")
			return ctrl.Result{}, fmt.Errorf("failed to ensure wanted Namespaces: %w", err)
		}
		if stopAndRetry {
			return ctrl.Result{
				RequeueAfter: defaultRetryAfterTime,
			}, nil
		}
	}

	// Phase 2.
	// Ensure unwanted namespaces are removed
	if err := r.ensureDeletionOfUnwantedNamespaces(ctx, addon); err != nil {
		log.Error(err, "failed to ensure deletion of unwanted namespaces")
		return ctrl.Result{}, fmt.Errorf("failed to ensure deletion of unwanted Namespaces: %w", err)
	}

	// Phase 3.
	// Ensure OperatorGroup
	{
		stop, err := r.ensureOperatorGroup(ctx, log, addon)
		if err != nil {
			log.Error(err, "failed to ensure operatorgroup")
			return ctrl.Result{}, fmt.Errorf("failed to ensure OperatorGroup: %w", err)
		}
		if stop {
			return ctrl.Result{}, nil
		}
	}

	// Phase 4.
	// Ensure CatalogSource for this Addon
	{
		// TODO dont pass logger, internal to Reconciler struct
		ensureResult, err := r.ensureCatalogSource(ctx, log, addon)
		if err != nil {
			log.Error(err, "failed to ensure catalogsource")
			return ctrl.Result{}, fmt.Errorf("failed to ensure CatalogSource: %w", err)
		}
		switch ensureResult {
		case ensureCatalogSourceResultRetry:
			return ctrl.Result{
				RequeueAfter: defaultRetryAfterTime,
			}, nil
		case ensureCatalogSourceResultStop:
			return ctrl.Result{}, nil
		}
	}

	// After last phase and if everything is healthy
	err = r.reportReadinessStatus(ctx, addon)
	if err != nil {
		log.Error(err, "failed to report readiness status")
		return ctrl.Result{}, fmt.Errorf("failed to report readiness status: %w", err)
	}

	return ctrl.Result{}, nil
}

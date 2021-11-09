package addon

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers/common"
	internalhandler "github.com/openshift/addon-operator/internal/handler"
)

const (
	cacheFinalizer = "addons.managed.openshift.io/cache"
)

type AddonReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	csvEventHandler csvEventHandler
	globalPause     bool
	globalPauseMux  sync.RWMutex
	addonRequeueCh  chan event.GenericEvent
}

// Pauses reconcilation of all Addon objects. Concurrency safe.
func (r *AddonReconciler) EnableGlobalPause(ctx context.Context) error {
	return r.setGlobalPause(ctx, true)
}

// Unpauses reconcilation of all Addon objects. Concurrency safe.
func (r *AddonReconciler) DisableGlobalPause(ctx context.Context) error {
	return r.setGlobalPause(ctx, false)
}

func (r *AddonReconciler) setGlobalPause(ctx context.Context, paused bool) error {
	r.globalPauseMux.Lock()
	defer r.globalPauseMux.Unlock()
	r.globalPause = paused

	if err := r.requeueAllAddons(ctx); err != nil {
		return fmt.Errorf("requeue all Addons: %w", err)
	}
	return nil
}

// requeue all addons that are currently in the local cache.
func (r *AddonReconciler) requeueAllAddons(ctx context.Context) error {
	addonList := &addonsv1alpha1.AddonList{}
	if err := r.List(ctx, addonList); err != nil {
		return fmt.Errorf("listing Addons, %w", err)
	}
	for i := range addonList.Items {
		r.addonRequeueCh <- event.GenericEvent{Object: &addonList.Items[i]}
	}
	return nil
}

type csvEventHandler interface {
	handler.EventHandler
	Free(addon *addonsv1alpha1.Addon)
	ReplaceMap(addon *addonsv1alpha1.Addon, csvKeys ...client.ObjectKey) (changed bool)
}

func (r *AddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.csvEventHandler = internalhandler.NewCSVEventHandler()
	r.addonRequeueCh = make(chan event.GenericEvent)
	return ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.Addon{}).
		Owns(&corev1.Namespace{}).
		Owns(&operatorsv1.OperatorGroup{}).
		Owns(&operatorsv1alpha1.CatalogSource{}).
		Owns(&operatorsv1alpha1.Subscription{}).
		Owns(&addonsv1alpha1.AddonInstance{}).
		Watches(&source.Kind{
			Type: &operatorsv1alpha1.ClusterServiceVersion{},
		}, r.csvEventHandler).
		Watches(&source.Channel{ // Requeue everything when entering/leaving global pause.
			Source: r.addonRequeueCh,
		}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

// AddonReconciler/Controller entrypoint
func (r *AddonReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("addon", req.NamespacedName.String())

	addon := &addonsv1alpha1.Addon{}
	err := r.Get(ctx, req.NamespacedName, addon)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// check for global pause
	r.globalPauseMux.RLock()
	defer r.globalPauseMux.RUnlock()
	if r.globalPause {
		err = r.reportAddonPauseStatus(ctx, addonsv1alpha1.AddonOperatorReasonPaused,
			addon)
		if err != nil {
			return ctrl.Result{}, err
		}
		// TODO: figure out how we can continue to report status
		return ctrl.Result{}, nil
	}

	// check for Addon pause
	if addon.Spec.Paused {
		err = r.reportAddonPauseStatus(ctx, addonsv1alpha1.AddonReasonPaused,
			addon)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Make sure Pause condition is removed
	if err := r.removeAddonPauseCondition(ctx, addon); err != nil {
		return ctrl.Result{}, nil
	}

	if !addon.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleAddonDeletion(ctx, addon)
	}

	// Phase 0.
	// Ensure cache finalizer
	if !controllerutil.ContainsFinalizer(addon, cacheFinalizer) {
		controllerutil.AddFinalizer(addon, cacheFinalizer)
		if err := r.Update(ctx, addon); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// Phase 1.
	// Ensure wanted namespaces
	if stopAndRetry, err := r.ensureWantedNamespaces(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure wanted Namespaces: %w", err)
	} else if stopAndRetry {
		return ctrl.Result{
			RequeueAfter: common.DefaultRetryAfterTime,
		}, nil
	}

	// Phase 2.
	// Ensure unwanted namespaces are removed
	if err := r.ensureDeletionOfUnwantedNamespaces(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure deletion of unwanted Namespaces: %w", err)
	}

	// Phase 3.
	// Ensure the creation of the corresponding AddonInstance in .spec.install.olmOwnNamespace/.spec.install.olmAllNamespaces namespace
	if err := r.ensureAddonInstance(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure the creation of addoninstance: %w", err)
	}

	// Phase 4.
	// Ensure OperatorGroup
	if stop, err := r.ensureOperatorGroup(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure OperatorGroup: %w", err)
	} else if stop {
		return ctrl.Result{}, nil
	}

	// Phase 5.
	ensureResult, catalogSource, err := r.ensureCatalogSource(ctx, log, addon)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure CatalogSource: %w", err)
	}
	switch ensureResult {
	case ensureCatalogSourceResultRetry:
		log.Info("requeuing", "reason", "catalogsource unready")
		return ctrl.Result{
			RequeueAfter: common.DefaultRetryAfterTime,
		}, nil
	case ensureCatalogSourceResultStop:
		return ctrl.Result{}, nil
	}

	// Phase 6.
	// Ensure Subscription for this Addon.
	currentCSVKey, requeue, err := r.ensureSubscription(
		ctx, log.WithName("phase-ensure-subscription"),
		addon, catalogSource)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure Subscription: %w", err)
	} else if requeue {
		return ctrl.Result{
			RequeueAfter: common.DefaultRetryAfterTime,
		}, nil
	}

	// Phase 7.
	// Observe current csv
	if requeue, err := r.observeCurrentCSV(ctx, addon, currentCSVKey); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to observe current CSV: %w", err)
	} else if requeue {
		log.Info("requeuing", "reason", "csv unready")
		return ctrl.Result{
			RequeueAfter: common.DefaultRetryAfterTime,
		}, nil
	}

	// After last phase and if everything is healthy
	if err = r.reportReadinessStatus(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to report readiness status: %w", err)
	}

	return ctrl.Result{}, nil
}

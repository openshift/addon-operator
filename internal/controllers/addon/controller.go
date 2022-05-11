package addon

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/metrics"

	"github.com/go-logr/logr"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	internalhandler "github.com/openshift/addon-operator/internal/controllers/addon/handler"
	"github.com/openshift/addon-operator/internal/ocm"
)

// Default timeout when we do a manual RequeueAfter
const (
	defaultRetryAfterTime = 10 * time.Second
	cacheFinalizer        = "addons.managed.openshift.io/cache"
)

type AddonReconciler struct {
	client.Client
	Log               logr.Logger
	Scheme            *runtime.Scheme
	Recorder          *metrics.Recorder
	UncachedClient    client.Client
	ClusterExternalID string
	// Namespace the AddonOperator is deployed into
	AddonOperatorNamespace string

	csvEventHandler csvEventHandler
	globalPause     bool
	globalPauseMux  sync.RWMutex
	addonRequeueCh  chan event.GenericEvent

	ocmClient    ocmClient
	ocmClientMux sync.RWMutex

	secretPropagationReconciler addonReconciler
	namespaceReconciler         addonReconciler
}

type addonReconciler interface {
	Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error)
}

func NewAddonReconciler(
	client client.Client,
	uncachedClient client.Client,
	log logr.Logger,
	scheme *runtime.Scheme,
	recorder *metrics.Recorder,
	clusterExternalID string,
	addonOperatorNamespace string,
) *AddonReconciler {
	return &AddonReconciler{
		Client:                 client,
		UncachedClient:         uncachedClient,
		Log:                    log,
		Scheme:                 scheme,
		Recorder:               recorder,
		ClusterExternalID:      clusterExternalID,
		AddonOperatorNamespace: addonOperatorNamespace,

		secretPropagationReconciler: &addonSecretPropagationReconciler{
			cachedClient:           client,
			uncachedClient:         uncachedClient,
			scheme:                 scheme,
			addonOperatorNamespace: addonOperatorNamespace,
		},

		namespaceReconciler: &namespaceReconciler{
			client: client,
			scheme: scheme,
		},
	}
}

type ocmClient interface {
	GetCluster(
		ctx context.Context,
		req ocm.ClusterGetRequest,
	) (res ocm.ClusterGetResponse, err error)
	PatchUpgradePolicy(
		ctx context.Context,
		req ocm.UpgradePolicyPatchRequest,
	) (res ocm.UpgradePolicyPatchResponse, err error)
}

func (r *AddonReconciler) InjectOCMClient(ctx context.Context, c *ocm.Client) error {
	r.ocmClientMux.Lock()
	defer r.ocmClientMux.Unlock()

	if r.ocmClient == nil {
		r.Log.Info("ocm client initialized for the first time")

		// Requeue all addons for the first time that the ocm client becomes available.
		if err := r.requeueAllAddons(ctx); err != nil {
			return fmt.Errorf("requeue all Addons: %w", err)
		}
	}

	r.ocmClient = c
	return nil
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
		Owns(&monitoringv1.ServiceMonitor{}).
		Watches(&source.Kind{
			Type: &corev1.Secret{},
		}, &handler.EnqueueRequestForOwner{
			OwnerType:    &addonsv1alpha1.Addon{},
			IsController: false, // We don't "control" the source secret, so we are only adding ourselves as owner/watcher
		}).
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
	ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := r.Log.WithValues("addon", req.NamespacedName.String())

	addon := &addonsv1alpha1.Addon{}
	if err := r.Get(ctx, req.NamespacedName, addon); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	defer func() {
		// Update metrics only if a Recorder is initialized
		if r.Recorder != nil {
			r.Recorder.RecordAddonMetrics(addon)
			r.Recorder.RecordAddonHealthInfo(addon, r.ClusterExternalID)
		}

		// Ensure we report to the UpgradePolicy endpoint, when we are done with whatever we are doing.
		if err != nil {
			return
		}
		err = r.handleUpgradePolicyStatusReporting(ctx, log, addon)

		// Finally, update the Status back to the kube-api
		// This is the only place where Status is being reported.
		if err != nil {
			return
		}
		err = r.Status().Update(ctx, addon)
	}()

	// Handle addon deletion before checking for pause condition.
	// This allows even paused addons to be deleted.
	if !addon.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.handleAddonDeletion(ctx, addon)
	}

	// check for global pause
	r.globalPauseMux.RLock()
	defer r.globalPauseMux.RUnlock()
	if r.globalPause {
		reportAddonPauseStatus(addon, addonsv1alpha1.AddonOperatorReasonPaused)
		// TODO: figure out how we can continue to report status
		return ctrl.Result{}, nil
	}

	// check for Addon pause
	if addon.Spec.Paused {
		reportAddonPauseStatus(addon, addonsv1alpha1.AddonReasonPaused)
		return ctrl.Result{}, nil
	}

	// Make sure Pause condition is removed
	r.removeAddonPauseCondition(addon)

	// Phase 0.
	// Ensure cache finalizer
	if !controllerutil.ContainsFinalizer(addon, cacheFinalizer) {
		controllerutil.AddFinalizer(addon, cacheFinalizer)
		if err := r.Update(ctx, addon); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// Reconcile Namespace
	if result, err := r.namespaceReconciler.Reconcile(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile namespace: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	// Reconcile addon pullsecrets
	if result, err := r.secretPropagationReconciler.Reconcile(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure pull secret: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	// TODO: encapsulate OLM phases into a new `OLMReconciler`

	// Phase 4.
	// Ensure the creation of the corresponding AddonInstance in .spec.install.olmOwnNamespace/.spec.install.olmAllNamespaces namespace
	if err := r.ensureAddonInstance(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure the creation of addoninstance: %w", err)
	}

	// Phase 5.
	// Ensure OperatorGroup
	if requeueResult, err := r.ensureOperatorGroup(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure OperatorGroup: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// Phase 6.
	var (
		catalogSource *operatorsv1alpha1.CatalogSource
		requeueResult requeueResult
	)
	if requeueResult, catalogSource, err = r.ensureCatalogSource(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure CatalogSource: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// Phase 7.
	if requeueResult, err = r.ensureAdditionalCatalogSources(ctx, log, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure additional CatalogSource: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// Phase 8.
	// Ensure Subscription for this Addon.
	requeueResult, currentCSVKey, err := r.ensureSubscription(
		ctx, log.WithName("phase-ensure-subscription"),
		addon, catalogSource)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure Subscription: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// Phase 9.
	// Observe current csv
	if requeueResult, err := r.observeCurrentCSV(ctx, addon, currentCSVKey); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to observe current CSV: %w", err)
	} else if requeueResult != resultNil {
		return handleExit(requeueResult), nil
	}

	// TODO: encapsulate monitoring operations into a new `monitoringReconciler`

	// Phase 10.
	// Possibly ensure monitoring federation
	// Normally this would be configured before the addon workload is installed
	// but currently the addon workload creates the monitoring stack by itself
	// thus we want to create the service monitor as late as possible to ensure that
	// cluster-monitoring prom does not try to scrape a non-existent addon prometheus.
	if err := r.ensureMonitoringFederation(ctx, addon); errors.Is(err, controllers.ErrNotOwnedByUs) {
		log.Info("stopping", "reason", "monitoring federation namespace or serviceMonitor owned by something else")

		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure ServiceMonitor: %w", err)
	}

	// Phase 11.
	// Remove possibly unwanted monitoring federation
	if err := r.ensureDeletionOfUnwantedMonitoringFederation(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure deletion of unwanted ServiceMonitors: %w", err)
	}

	// After last phase and if everything is healthy
	reportReadinessStatus(addon)
	return ctrl.Result{}, nil
}

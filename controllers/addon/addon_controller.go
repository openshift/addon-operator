package addon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"

	"github.com/go-logr/logr"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	internalhandler "github.com/openshift/addon-operator/controllers/addon/handler"
	"github.com/openshift/addon-operator/internal/ocm"
)

const (
	// Default timeout when we do a manual RequeueAfter
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

	operatorResourceHandler    operatorResourceHandler
	globalPause                bool
	globalPauseMux             sync.RWMutex
	statusReportingEnabled     bool
	upgradePolicyStatusEnabled bool
	addonRequeueCh             chan event.GenericEvent

	ocmClient    ocmClient
	ocmClientMux sync.RWMutex

	// List of Addon sub-reconcilers.
	// Reconcilers will run  serially
	// in the order in which they appear in this slice.
	subReconcilers []addonReconciler
}

type addonHealth struct {
	reason string
}

func (a addonHealth) GetReason() string {
	return a.reason
}

var (
	UnschedulableAddonPod = addonHealth{reason: "UnschedulableAddonPod"}
)

type addonReconciler interface {
	Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error)
	Name() string
}

func NewAddonReconciler(
	client client.Client,
	uncachedClient client.Client,
	log logr.Logger,
	scheme *runtime.Scheme,
	recorder *metrics.Recorder,
	clusterExternalID string,
	addonOperatorNamespace string,
	enableStatusReporting bool,
	enableUpgradePolicyStatus bool,
	opts ...AddonReconcilerOptions,
) *AddonReconciler {
	operatorResourceHandler := internalhandler.NewOperatorResourceHandler()
	adoReconciler := &AddonReconciler{
		Client:                     client,
		UncachedClient:             uncachedClient,
		Log:                        log,
		Scheme:                     scheme,
		Recorder:                   recorder,
		ClusterExternalID:          clusterExternalID,
		AddonOperatorNamespace:     addonOperatorNamespace,
		operatorResourceHandler:    operatorResourceHandler,
		statusReportingEnabled:     enableStatusReporting,
		upgradePolicyStatusEnabled: enableUpgradePolicyStatus,
		subReconcilers: []addonReconciler{
			// Step 1: Check if addon is being deleted.
			&addonDeletionReconciler{
				clock: defaultClock{},
				handlers: []addonDeletionHandler{
					&legacyDeletionHandler{client: client, uncachedClient: uncachedClient},
					&addonInstanceDeletionHandler{client: client},
				},
				recorder: recorder,
			},
			// Step 2: Reconcile Namespace
			&namespaceReconciler{
				client:   client,
				scheme:   scheme,
				recorder: recorder,
			},
			// Step 3: Reconcile Addon pull secrets
			&addonSecretPropagationReconciler{
				cachedClient:           client,
				uncachedClient:         uncachedClient,
				scheme:                 scheme,
				addonOperatorNamespace: addonOperatorNamespace,
				recorder:               recorder,
			},
			// Step 4: Reconcile AddonInstance object
			&addonInstanceReconciler{
				client:   client,
				scheme:   scheme,
				recorder: recorder,
			},
			// Step 5: Reconcile OLM objects
			&olmReconciler{
				client:                  client,
				uncachedClient:          uncachedClient,
				scheme:                  scheme,
				operatorResourceHandler: operatorResourceHandler,
				recorder:                recorder,
			},
			// Step 6: Reconcile Monitoring Federation
			&monitoringFederationReconciler{
				client:   client,
				scheme:   scheme,
				recorder: recorder,
			},
		},
	}

	for _, opt := range opts {
		opt.ApplyToAddonReconciler(adoReconciler)
	}
	return adoReconciler
}

type ocmClient interface {
	GetClusterIDAndName() (string, string)
	GetCluster(
		ctx context.Context,
		req ocm.ClusterGetRequest,
	) (res ocm.ClusterGetResponse, err error)
	PatchUpgradePolicy(
		ctx context.Context,
		req ocm.UpgradePolicyPatchRequest,
	) (res ocm.UpgradePolicyPatchResponse, err error)
	GetUpgradePolicy(
		ctx context.Context,
		req ocm.UpgradePolicyGetRequest,
	) (res ocm.UpgradePolicyGetResponse, err error)
	PostAddOnStatus(
		ctx context.Context,
		req ocm.AddOnStatusPostRequest,
	) (res ocm.AddOnStatusResponse, err error)
	PatchAddOnStatus(
		ctx context.Context,
		addonID string,
		req ocm.AddOnStatusPatchRequest,
	) (res ocm.AddOnStatusResponse, err error)
	GetAddOnStatus(
		ctx context.Context,
		addonID string,
	) (res ocm.AddOnStatusResponse, err error)
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

func (r *AddonReconciler) GetOCMClusterInfo() OcmClusterInfo {
	r.ocmClientMux.RLock()
	defer r.ocmClientMux.RUnlock()

	if r.ocmClient == nil {
		return OcmClusterInfo{}
	}
	clusterId, clusterName := r.ocmClient.GetClusterIDAndName()
	return OcmClusterInfo{
		ID:   clusterId,
		Name: clusterName,
	}
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

type operatorResourceHandler interface {
	handler.EventHandler
	Free(addon *addonsv1alpha1.Addon)
	UpdateMap(addon *addonsv1alpha1.Addon, operatorKey client.ObjectKey) (changed bool)
}

func (r *AddonReconciler) SetupWithManager(mgr ctrl.Manager, opts ...AddonReconcilerOptions) error {
	if r.operatorResourceHandler == nil {
		return fmt.Errorf("operatorResourceHandler cannot be nil")
	}

	r.addonRequeueCh = make(chan event.GenericEvent)
	adoControllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.Addon{}).
		Owns(&corev1.Namespace{}).
		Owns(&operatorsv1.OperatorGroup{}).
		Owns(&operatorsv1alpha1.CatalogSource{}).
		Owns(&operatorsv1alpha1.Subscription{}).
		Owns(&addonsv1alpha1.AddonInstance{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		WatchesRawSource(source.Kind(
			mgr.GetCache(), &corev1.Secret{}),
			handler.EnqueueRequestForOwner(r.Scheme, mgr.GetRESTMapper(), &addonsv1alpha1.Addon{})).
		WatchesRawSource(source.Kind(
			mgr.GetCache(), &operatorsv1.Operator{}),
			r.operatorResourceHandler, builder.OnlyMetadata,
		).
		WatchesRawSource(&source.Channel{ // Requeue everything when entering/leaving global pause.
			Source: r.addonRequeueCh,
		}, &handler.EnqueueRequestForObject{})

	for _, opt := range opts {
		opt.ApplyToControllerBuilder(adoControllerBuilder)
	}

	return adoControllerBuilder.Complete(r)
}

// AddonReconciler/Controller entrypoint
func (r *AddonReconciler) Reconcile(
	ctx context.Context, req ctrl.Request,
) (ctrl.Result, error) {
	logger := r.Log.WithValues("addon", req.NamespacedName.String())
	ctx = controllers.ContextWithLogger(ctx, logger)
	reconErr := metrics.NewReconcileError("addon", r.Recorder, false)

	addon := &addonsv1alpha1.Addon{}
	if err := r.Get(ctx, req.NamespacedName, addon); err != nil {
		reconErr.Report(controllers.ErrGetAddon, addon.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconcileResult, reconcileErr := r.reconcile(ctx, addon, logger)

	if err := r.recordAddonMetrics(ctx, addon); err != nil {
		r.Log.Error(err, "failed to record addon metrics")
	}

	multiErr := r.syncWithExternalAPIs(ctx, logger, addon)

	if multiErr.ErrorOrNil() != nil {
		var ocmErr ocm.OCMError
		if errors.As(multiErr, &ocmErr) {
			reconErr.Report(controllers.ErrOCMClientRequest, addon.Name)
		} else {
			reconErr.Report(controllers.ErrSyncWithExternalAPIs, addon.Name)
		}
	}

	// append reconcilerErr
	multiErr = multierror.Append(multiErr, reconcileErr)

	// We report the observed version regardless of whether the addon
	// is available or not.
	reportObservedVersion(addon)

	if statusErr := r.Status().Update(ctx, addon); statusErr != nil {
		multiErr = multierror.Append(multiErr, statusErr)
		return reconcile.Result{}, multiErr
	}
	return reconcileResult, multiErr.ErrorOrNil()
}

func (r *AddonReconciler) syncWithExternalAPIs(ctx context.Context, logger logr.Logger, addon *addonsv1alpha1.Addon) *multierror.Error {
	// We don't immeadiately return on errors, we append them to a multi-error object.
	var multiErr *multierror.Error

	upgradePolicyErr := r.handleUpgradePolicyStatusReporting(
		ctx, logger.WithName("UpgradePolicyStatusReporter"), addon,
	)
	multiErr = multierror.Append(multiErr, upgradePolicyErr)

	ocmStatusReportingErr := r.handleOCMAddOnStatusReporting(
		ctx, logger.WithName("AddonStatusReporter"), addon,
	)
	multiErr = multierror.Append(multiErr, ocmStatusReportingErr)

	return multiErr
}

func (r *AddonReconciler) reconcile(ctx context.Context, addon *addonsv1alpha1.Addon,
	log logr.Logger,
) (ctrl.Result, error) {
	ctx = controllers.ContextWithLogger(ctx, log)
	reconErr := metrics.NewReconcileError("addon", r.Recorder, false)
	subReconErr := metrics.NewReconcileError("addon", r.Recorder, true)
	// Handle addon deletion before checking for pause condition.
	// This allows even paused addons to be deleted.
	if !addon.DeletionTimestamp.IsZero() {
		return r.handleAddonCRDeletion(ctx, addon)
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

	// Check if the addon is being upgraded
	// by comparing spec.version and status.ObservedVersion.
	if addonIsBeingUpgraded(addon) {
		reportAddonUpgradeStarted(addon)
		return ctrl.Result{}, nil
	}

	// Set installed condition to false if its not already present.
	if installedConditionMissing(addon) {
		reportInstalledConditionFalse(addon)
	}

	// Ensure cache finalizer
	if !controllerutil.ContainsFinalizer(addon, cacheFinalizer) {
		controllerutil.AddFinalizer(addon, cacheFinalizer)
		if err := r.Update(ctx, addon); err != nil {
			reconErr.Report(controllers.ErrUpdateAddon, addon.Name)
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// Run each sub reconciler serially
	for _, reconciler := range r.subReconcilers {
		if result, err := reconciler.Reconcile(ctx, addon); err != nil {
			subReconErr.Report(err, addon.Name)
			return ctrl.Result{}, fmt.Errorf("%s : failed to reconcile : %w", reconciler.Name(), err)
		} else if !result.IsZero() {
			return result, nil
		}
	}

	return ctrl.Result{}, nil
}

// Lists and filters pods with corev1.PodReasonUnschedulable status
func (r *AddonReconciler) listUnschedulableAddonPods(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
) (*corev1.PodList, error) {
	addonPods := &corev1.PodList{}
	unschedulablePods := &corev1.PodList{}

	var targetNs string
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMAllNamespaces:
		if addon.Spec.Install.OLMAllNamespaces != nil {
			targetNs = addon.Spec.Install.OLMAllNamespaces.Namespace
		}
	case addonsv1alpha1.OLMOwnNamespace:
		if addon.Spec.Install.OLMOwnNamespace != nil {
			targetNs = addon.Spec.Install.OLMOwnNamespace.Namespace
		}
	}

	if len(strings.TrimSpace(targetNs)) == 0 {
		return unschedulablePods, errors.New("failed to get addon's namespace")
	}

	if err := r.Client.List(
		ctx,
		addonPods,
		client.InNamespace(targetNs),
	); err != nil {
		return unschedulablePods, fmt.Errorf("failed listing addon pods: %w", err)
	}

	for _, pod := range addonPods.Items {
		for _, podCond := range pod.Status.Conditions {
			if podCond.Type == corev1.PodScheduled &&
				podCond.Reason == corev1.PodReasonUnschedulable &&
				podCond.Status == corev1.ConditionFalse {
				unschedulablePods.Items = append(unschedulablePods.Items, pod)
			}
		}
	}
	return unschedulablePods, nil

}

// Gathers addon data for metric collection
func (r *AddonReconciler) recordAddonMetrics(
	ctx context.Context,
	addon *addonsv1alpha1.Addon) (err error) {

	if r.Recorder == nil {
		return
	}

	unschedPods := &corev1.PodList{}
	if availableCond := meta.FindStatusCondition(
		addon.Status.Conditions,
		addonsv1alpha1.Available,
	); availableCond != nil && availableCond.Status == metav1.ConditionFalse {
		unschedPods, err = r.listUnschedulableAddonPods(ctx, addon)
		if err != nil {
			return err
		}
	}

	health := addonHealth{}
	if len(unschedPods.Items) > 0 {
		health = UnschedulableAddonPod
	}

	r.Recorder.RecordAddonMetrics(
		addon,
		health,
	)
	return
}

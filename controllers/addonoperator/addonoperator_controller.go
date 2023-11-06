package addonoperator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openshift/addon-operator/internal/metrics"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/ocm"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	defaultAddonOperatorRequeueTime = time.Minute
)

type AddonOperatorReconciler struct {
	client.Client
	UncachedClient      client.Client
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	GlobalPauseManager  globalPauseManager
	OCMClientManager    ocmClientManager
	Recorder            *metrics.Recorder
	ClusterExternalID   string
	FeatureTogglesState []string // no need to guard this with a mutex considering the fact that no two goroutines would ever try to update it as this is only initialized at startup
}

func (r *AddonOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.AddonOperator{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WatchesRawSource(source.Func(enqueueAddonOperator), &handler.EnqueueRequestForObject{}).
		Complete(r)
}

func enqueueAddonOperator(ctx context.Context, h handler.EventHandler,
	q workqueue.RateLimitingInterface, p ...predicate.Predicate) error {
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name: addonsv1alpha1.DefaultAddonOperatorName,
	}})

	return nil
}

func (r *AddonOperatorReconciler) Reconcile(
	ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("addon-operator", req.NamespacedName.String())

	addonOperator := &addonsv1alpha1.AddonOperator{}
	err := r.Get(ctx, client.ObjectKey{
		Name: addonsv1alpha1.DefaultAddonOperatorName,
	}, addonOperator)

	defer func() {
		// update metrics
		if r.Recorder != nil {
			r.Recorder.SetAddonOperatorPaused(meta.IsStatusConditionTrue(
				addonOperator.Status.Conditions, addonsv1alpha1.AddonOperatorPaused))
		}
	}()

	reconErr := metrics.NewReconcileError("addonoperator", r.Recorder, false)

	// Create default AddonOperator object if it doesn't exist
	if apierrors.IsNotFound(err) {
		log.Info("default AddonOperator not found")
		reconErr.Report(controllers.ErrGetDefaultAddonOperator, addonsv1alpha1.DefaultAddonOperatorName)
		return ctrl.Result{}, r.handleAddonOperatorCreation(ctx, log)
	}
	if err != nil {
		reconErr.Report(controllers.ErrGetDefaultAddonOperator, addonsv1alpha1.DefaultAddonOperatorName)
		return ctrl.Result{}, err
	}

	// Exiting here so that k8s can restart ADO (pods).
	// This will make ADO bootstrap itself w.r.t to the latest state of feature toggles in the cluster (AddonOperator CR).
	if !areSlicesEquivalent(r.FeatureTogglesState, strings.Split(addonOperator.Spec.FeatureFlags, ",")) {
		log.Info("found a different state of feature toggles, exiting AddonOperator")
		os.Exit(0)
	}

	if err := r.handleGlobalPause(ctx, addonOperator); err != nil {
		reconErr.Report(controllers.ErrAddonOperatorHandleGlobalPause, addonOperator.Name)
		return ctrl.Result{}, fmt.Errorf("handling global pause: %w", err)
	}

	if err := r.handleOCMClient(ctx, log, addonOperator); err != nil {
		reconErr.Report(controllers.ErrCreateOCMClient, addonOperator.Name)
		return ctrl.Result{}, fmt.Errorf("handling OCM client: %w", err)
	}

	// TODO: This is where all the checking / validation happens
	// for "in-depth" status reporting

	err = r.reportAddonOperatorReadinessStatus(ctx, addonOperator)
	if err != nil {
		reconErr.Report(controllers.ErrReportAddonOperatorStatus, addonOperator.Name)
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: defaultAddonOperatorRequeueTime}, nil
}

func areSlicesEquivalent(sliceA []string, sliceB []string) bool {
	if len(sliceA) != len(sliceB) {
		return false
	}
	elementsDiffTracker := map[string]int{}
	for _, strA := range sliceA {
		elementsDiffTracker[strA] += 1
	}
	for _, strB := range sliceB {
		elementsDiffTracker[strB] -= 1
	}

	// if elementsDiffTracker as 0 value for all elements, this means that for every string in slice A there was a string in Slice B proving their equivalence
	for _, diff := range elementsDiffTracker {
		if diff != 0 {
			return false
		}
	}

	return true
}

// Creates an OCM API client and injects it into the OCM Client Manager for distribution.
func (r *AddonOperatorReconciler) handleOCMClient(
	ctx context.Context, log logr.Logger, addonOperator *addonsv1alpha1.AddonOperator) error {
	if addonOperator.Spec.OCM == nil {
		return nil
	}

	secret := &corev1.Secret{}
	// Use an uncached client to get this secret,
	// so we don't setup a cluster-wide cache for Secrets.
	// Saving memory and required RBAC privileges.
	if err := r.UncachedClient.Get(ctx, client.ObjectKey{
		Name:      addonOperator.Spec.OCM.Secret.Name,
		Namespace: addonOperator.Spec.OCM.Secret.Namespace,
	}, secret); err != nil {
		return fmt.Errorf("getting ocm secret: %w", err)
	}

	accessToken, err := accessTokenFromDockerConfig(secret.Data[corev1.DockerConfigJsonKey])
	if err != nil {
		return fmt.Errorf("extracting access token from .dockerconfigjson: %w", err)
	}

	c, _ := ocm.NewClient(
		ctx,
		ocm.WithEndpoint(addonOperator.Spec.OCM.Endpoint),
		ocm.WithAccessToken(accessToken),
		ocm.WithClusterExternalID(r.ClusterExternalID),
	)

	//ocm client not initialized, usually because the OCM API is not yet
	//available or because the ClusterID from the ClusterVersion doesn't
	//properly translate into an internal_id
	if c == nil {
		log.Info("delaying ocm client initialization until the OCM API is available")
		return nil
	}

	if err := r.OCMClientManager.InjectOCMClient(ctx, c); err != nil {
		return fmt.Errorf("injecting ocm client: %w", err)
	}
	return nil
}

func (r *AddonOperatorReconciler) handleGlobalPause(
	ctx context.Context, addonOperator *addonsv1alpha1.AddonOperator) error {
	// Check if addonoperator.spec.paused == true
	if addonOperator.Spec.Paused {
		// Check if Paused condition has already been reported
		if meta.IsStatusConditionTrue(addonOperator.Status.Conditions,
			addonsv1alpha1.AddonOperatorPaused) {
			return nil
		}
		if err := r.GlobalPauseManager.EnableGlobalPause(ctx); err != nil {
			return fmt.Errorf("setting global pause: %w", err)
		}
		if err := r.reportAddonOperatorPauseStatus(ctx, addonOperator); err != nil {
			return fmt.Errorf("report AddonOperator paused: %w", err)
		}

		return nil
	}

	// Unpause only if the current reported condition is Paused
	if !meta.IsStatusConditionTrue(addonOperator.Status.Conditions,
		addonsv1alpha1.AddonOperatorPaused) {
		return nil
	}
	if err := r.GlobalPauseManager.DisableGlobalPause(ctx); err != nil {
		return fmt.Errorf("removing global pause: %w", err)
	}
	if err := r.removeAddonOperatorPauseCondition(ctx, addonOperator); err != nil {
		return fmt.Errorf("remove AddonOperator paused: %w", err)
	}
	return nil
}

func accessTokenFromDockerConfig(dockerConfigJson []byte) (string, error) {
	dockerConfig := map[string]interface{}{}
	if err := json.Unmarshal(dockerConfigJson, &dockerConfig); err != nil {
		return "", fmt.Errorf("unmarshalling docker config json: %w", err)
	}

	accessToken, ok, err := unstructured.NestedString(
		dockerConfig, "auths", "cloud.openshift.com", "auth")
	if err != nil {
		return "", fmt.Errorf("accessing cloud.openshift.com auth key: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("missing token for cloud.openshift.com")
	}
	return accessToken, nil
}

package addon

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"
)

const ADDON_INSTANCE_RECONCILER_NAME = "addonInstanceReconciler"

type addonInstanceReconciler struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder *metrics.Recorder
}

func (r *addonInstanceReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	reconErr := metrics.NewReconcileError("addon", r.recorder, true)
	// Ensure the creation of the corresponding AddonInstance in .spec.install.olmOwnNamespace/.spec.install.olmAllNamespaces namespace
	if err := r.ensureAddonInstance(ctx, addon); err != nil {
		err = reconErr.Join(err, controllers.ErrEnsureCreateAddonInstance)
		return resultNil, err
	}
	return resultNil, nil
}

func (r *addonInstanceReconciler) Name() string {
	return ADDON_INSTANCE_RECONCILER_NAME
}

func (r *addonInstanceReconciler) Order() subReconcilerOrder {
	return AddonInstanceReconcilerOrder
}

// Ensures the presence of an AddonInstance well-compliant with the provided Addon object
func (r *addonInstanceReconciler) ensureAddonInstance(
	ctx context.Context, addon *addonsv1alpha1.Addon) (err error) {
	log := controllers.LoggerFromContext(ctx)
	// not capturing "stop" because it won't ever be reached due to the guard rails of CRD Enum-Validation Markers
	commonConfig, stop := parseAddonInstallConfig(log, addon)
	if stop {
		return fmt.Errorf("failed to create addonInstance due to misconfigured install.spec.type")
	}

	desiredAddonInstance := &addonsv1alpha1.AddonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addonsv1alpha1.DefaultAddonInstanceName,
			Namespace: commonConfig.Namespace,
		},
		// Can't skip specifying spec because in this case, the zero-value for metav1.Duration will be perceived beforehand i.e. 0s instead of CRD's default value of 10s
		Spec: addonsv1alpha1.AddonInstanceSpec{
			HeartbeatUpdatePeriod: metav1.Duration{
				Duration: addonsv1alpha1.DefaultAddonInstanceHeartbeatUpdatePeriod,
			},
		},
	}

	if err := controllerutil.SetControllerReference(addon, desiredAddonInstance, r.scheme); err != nil {
		return fmt.Errorf("setting controller reference: %w", err)
	}

	return r.reconcileAddonInstance(ctx, desiredAddonInstance)
}

// Reconciles the reality to have the desired AddonInstance resource by creating it if it does not exist,
// or updating if it exists with a different spec.
func (r *addonInstanceReconciler) reconcileAddonInstance(
	ctx context.Context, desiredAddonInstance *addonsv1alpha1.AddonInstance) error {
	currentAddonInstance := &addonsv1alpha1.AddonInstance{}
	err := r.client.Get(ctx, client.ObjectKeyFromObject(desiredAddonInstance), currentAddonInstance)
	if apiErrors.IsNotFound(err) {
		return r.client.Create(ctx, desiredAddonInstance)
	}
	if err != nil {
		return fmt.Errorf("getting AddonInstance: %w", err)
	}
	// We don't want to overwrite the marked for deletion field of the existing
	// addoninstance. The addon deletion sub-reconciler handles that part.
	desiredAddonInstance.Spec.MarkedForDeletion = currentAddonInstance.Spec.MarkedForDeletion
	if !equality.Semantic.DeepEqual(currentAddonInstance.Spec, desiredAddonInstance.Spec) {
		currentAddonInstance.Spec = desiredAddonInstance.Spec
		currentAddonInstance.OwnerReferences = desiredAddonInstance.OwnerReferences
		return r.client.Update(ctx, currentAddonInstance)
	}
	return nil
}

package addon

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

type addonInstanceReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func (r *addonInstanceReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (reconcile.Result, error) {
	// Ensure the creation of the corresponding AddonInstance in .spec.install.olmOwnNamespace/.spec.install.olmAllNamespaces namespace
	if err := r.ensureAddonInstance(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure the creation of addoninstance: %w", err)
	}
	return reconcile.Result{}, nil
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
			HeartbeatUpdatePeriod: controllers.DefaultAddonInstanceHeartbeatUpdatePeriod,
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
	ctx context.Context, addonInstance *addonsv1alpha1.AddonInstance) error {
	currentAddonInstance := &addonsv1alpha1.AddonInstance{}
	err := r.client.Get(ctx, client.ObjectKeyFromObject(addonInstance), currentAddonInstance)
	if errors.IsNotFound(err) {
		return r.client.Create(ctx, addonInstance)
	}
	if err != nil {
		return fmt.Errorf("getting AddonInstance: %w", err)
	}
	if !equality.Semantic.DeepEqual(currentAddonInstance.Spec, addonInstance.Spec) {
		return r.client.Update(ctx, addonInstance)
	}
	return nil
}

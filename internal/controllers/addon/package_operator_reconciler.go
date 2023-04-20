package addon

import (
	"context"
	"fmt"

	"github.com/openshift/addon-operator/internal/controllers"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"

	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const pkgTemplate = `
apiVersion: "%s"
kind: ClusterPackage
metadata:
  name: "%s"
  namespace: "%s"
spec:
  image: "%s"
  config: {{toJson .config}}
`
const packageOperatorName = "packageOperatorReconciler"

type PackageOperatorReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (r *PackageOperatorReconciler) Name() string { return packageOperatorName }

func (r *PackageOperatorReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if addon.Spec.AddonPackageOperator == nil {
		return ctrl.Result{}, r.ensureClusterObjectTemplateTornDown(ctx, addon)
	}
	return r.reconcileClusterObjectTemplate(ctx, addon)
}

func (r *PackageOperatorReconciler) reconcileClusterObjectTemplate(ctx context.Context, addon *addonsv1alpha1.Addon) (res ctrl.Result, err error) {
	logger := controllers.LoggerFromContext(ctx)
	defer func() {
		if err == nil {
			logger.Info("successfully reconciled")
		}
	}()
	clusterObjectTemplate := &pkov1alpha1.ClusterObjectTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name,
			Namespace: addon.Namespace,
		},
		Spec: pkov1alpha1.ObjectTemplateSpec{
			Template: fmt.Sprintf(pkgTemplate,
				pkov1alpha1.GroupVersion,
				addon.Name,
				addon.Namespace,
				addon.Spec.AddonPackageOperator.Image,
			),
			Sources: []pkov1alpha1.ObjectTemplateSource{},
		},
	}

	if err = controllerutil.SetControllerReference(addon, clusterObjectTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting owner reference: %w", err)
	}

	existingClusterObjectTemplate, err := r.getExistingClusterObjectTemplate(ctx, addon)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Client.Create(ctx, clusterObjectTemplate); err != nil {
				return ctrl.Result{}, fmt.Errorf("creating ClusterObjectTemplate object: %w", err)
			}
		}
		return ctrl.Result{}, fmt.Errorf("getting ClusterObjectTemplate object: %w", err)
	}

	clusterObjectTemplate.ResourceVersion = existingClusterObjectTemplate.ResourceVersion
	if err := r.Client.Update(ctx, clusterObjectTemplate); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating ClusterObjectTemplate object: %w", err)
	}

	if packageAvailable := r.updateAddonStatus(addon, existingClusterObjectTemplate); !packageAvailable {
		return handleExit(resultRetry), nil
	}

	return ctrl.Result{}, nil
}

func (r *PackageOperatorReconciler) updateAddonStatus(addon *addonsv1alpha1.Addon, clusterObjectTemplate *pkov1alpha1.ClusterObjectTemplate) bool {
	availableCondition := meta.FindStatusCondition(clusterObjectTemplate.Status.Conditions, pkov1alpha1.PackageAvailable)
	if availableCondition != nil &&
		availableCondition.ObservedGeneration == clusterObjectTemplate.GetGeneration() &&
		availableCondition.Status == metav1.ConditionTrue {
		return true
	}

	reportUnreadyClusterObjectTemplate(addon)
	return false

}

func (r *PackageOperatorReconciler) ensureClusterObjectTemplateTornDown(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	existing, err := r.getExistingClusterObjectTemplate(ctx, addon)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting ClusterObjectTemplate object: %w", err)
	}
	if err = r.Client.Delete(ctx, existing); err != nil {
		return fmt.Errorf("deleting ClusterObjectTemplate object: %w", err)
	}
	return nil
}

func (r *PackageOperatorReconciler) getExistingClusterObjectTemplate(ctx context.Context, addon *addonsv1alpha1.Addon) (*pkov1alpha1.ClusterObjectTemplate, error) {
	existing := &pkov1alpha1.ClusterObjectTemplate{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: addon.Namespace, Name: addon.Name}, existing)
	return existing, err
}

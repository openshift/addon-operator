package addon

import (
	"context"
	"fmt"

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
	return ctrl.Result{}, r.reconcileClusterObjectTemplate(ctx, addon)
}

func (r *PackageOperatorReconciler) reconcileClusterObjectTemplate(ctx context.Context, addon *addonsv1alpha1.Addon) error {
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

	if err := controllerutil.SetControllerReference(addon, clusterObjectTemplate, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference: %w", err)
	}

	existing, err := r.getExistingClusterObjectTemplate(ctx, addon)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Client.Create(ctx, clusterObjectTemplate); err != nil {
				return fmt.Errorf("creating ClusterObjectTemplate object: %w", err)
			}
		}
		return fmt.Errorf("getting ClusterObjectTemplate object: %w", err)
	}

	r.updateAddonStatusConditionsFromPackage(addon, clusterObjectTemplate)

	if err := r.Client.Patch(ctx, existing, client.MergeFrom(clusterObjectTemplate)); err != nil {
		return fmt.Errorf("updating ClusterObjectTemplate object: %w", err)
	}

	return nil
}

func (r *PackageOperatorReconciler) updateAddonStatusConditionsFromPackage(addon *addonsv1alpha1.Addon, clusterObjectTemplate *pkov1alpha1.ClusterObjectTemplate) {
	for _, cond := range clusterObjectTemplate.Status.Conditions {
		if clusterObjectTemplate.GetGeneration() != cond.ObservedGeneration {
			// condition is out of date, don't copy it over
			continue
		}

		newCond := metav1.Condition{
			Type:               cond.Type,
			Status:             cond.Status,
			ObservedGeneration: addon.Generation,
			Reason:             cond.Reason,
			Message:            cond.Message,
		}
		meta.SetStatusCondition(&addon.Status.Conditions, newCond)
	}
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

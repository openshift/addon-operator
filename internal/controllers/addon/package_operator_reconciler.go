package addon

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
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
kind: Package
metadata:
  name: "%s"
  namespace: "%s"
spec:
  image: "%s"
  config: {{toJSON .sources}}
`
const packageOperatorName = "packageOperatorReconciler"

type PackageOperatorReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (r *PackageOperatorReconciler) Name() string { return packageOperatorName }

func (r *PackageOperatorReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if addon.Spec.AddonPackageOperator == nil {
		return ctrl.Result{}, nil
	}

	pkg := &pkov1alpha1.ClusterObjectTemplate{
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

	if err := controllerutil.SetControllerReference(addon, &pkg.ObjectMeta, r.Scheme); err != nil {
		panic(fmt.Errorf("set owner reference: %w", err))
	}

	existing := &pkov1alpha1.ClusterObjectTemplate{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: addon.Namespace, Name: addon.Name}, existing)
	switch {
	case err == nil:
		if err := r.Client.Patch(ctx, existing, client.MergeFrom(pkg)); err != nil {
			return ctrl.Result{}, fmt.Errorf("update pko object: %w", err)
		}
	case errors.IsNotFound(err):
		if err := r.Client.Create(ctx, pkg); err != nil {
			return ctrl.Result{}, fmt.Errorf("create pko object: %w", err)
		}
	default:
		return ctrl.Result{}, fmt.Errorf("get pko object: %w", err)
	}

	return ctrl.Result{}, nil
}

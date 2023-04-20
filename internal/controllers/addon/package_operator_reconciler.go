package addon

import (
	"context"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
spec:
  image: "%s"
  config:
    addons:
      v1alpha1:
        deadMansSnitchUrl: {{.config%s | b64dec}}
        pagerDutyKey: {{.config%s | b64dec}}
`

const packageOperatorName = "packageOperatorReconciler"
const deadMansSnitchUrlConfigKey = ".deadMansSnitchUrl"
const pagerDutyKeyConfigKey = ".pagerDutyKey"

type PackageOperatorReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (r *PackageOperatorReconciler) Name() string { return packageOperatorName }

func (r *PackageOperatorReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if addon.Spec.AddonPackageOperator == nil {
		return ctrl.Result{}, r.makeSureClusterObjectTemplateDoesNotExist(ctx, addon)
	}
	return ctrl.Result{}, r.makeSureClusterObjectTemplateExists(ctx, addon)
}

func (r *PackageOperatorReconciler) makeSureClusterObjectTemplateExists(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	if len(addon.Spec.Namespaces) < 1 {
		return errors.New(fmt.Sprintf("no namespace configured in addon %s", addon.Name))
	}

	addonDestNamespace := addon.Spec.Namespaces[0].Name

	templateString := fmt.Sprintf(pkgTemplate,
		pkov1alpha1.GroupVersion,
		addon.Name,
		addon.Spec.AddonPackageOperator.Image,
		deadMansSnitchUrlConfigKey,
		pagerDutyKeyConfigKey,
	)

	pkg := &pkov1alpha1.ClusterObjectTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: addon.Name,
		},
		Spec: pkov1alpha1.ObjectTemplateSpec{
			Template: templateString,
			Sources: []pkov1alpha1.ObjectTemplateSource{
				{
					Optional:   true,
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       addon.Name + "-deadmanssnitch",
					Namespace:  addonDestNamespace,
					Items: []pkov1alpha1.ObjectTemplateSourceItem{
						{
							Key:         ".data.SNITCH_URL",
							Destination: deadMansSnitchUrlConfigKey,
						},
					},
				},
				{
					Optional:   true,
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       addon.Name + "-pagerduty",
					Namespace:  addonDestNamespace,
					Items: []pkov1alpha1.ObjectTemplateSourceItem{
						{
							Key:         ".data.PAGERDUTY_KEY",
							Destination: pagerDutyKeyConfigKey,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(addon, &pkg.ObjectMeta, r.Scheme); err != nil {
		panic(fmt.Errorf("set owner reference: %w", err))
	}

	existing, err := r.getExisting(ctx, addon)
	switch {
	case err == nil:
		if err := r.Client.Patch(ctx, existing, client.MergeFrom(pkg)); err != nil {
			return fmt.Errorf("update pko object: %w", err)
		}
	case k8serrors.IsNotFound(err):
		if err := r.Client.Create(ctx, pkg); err != nil {
			return fmt.Errorf("create pko object: %w", err)
		}
	default:
		return fmt.Errorf("get pko object: %w", err)
	}
	return nil
}

func (r *PackageOperatorReconciler) makeSureClusterObjectTemplateDoesNotExist(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	existing, err := r.getExisting(ctx, addon)
	switch {
	case err == nil:
		if err := r.Client.Delete(ctx, existing); err != nil {
			return fmt.Errorf("delete pko object: %w", err)
		}
	case k8serrors.IsNotFound(err):
		return nil
	default:
		return fmt.Errorf("get pko object: %w", err)
	}
	return nil
}

func (r *PackageOperatorReconciler) getExisting(ctx context.Context, addon *addonsv1alpha1.Addon) (*pkov1alpha1.ClusterObjectTemplate, error) {
	existing := &pkov1alpha1.ClusterObjectTemplate{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: addon.Namespace, Name: addon.Name}, existing)
	return existing, err
}

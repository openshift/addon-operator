package addon

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
  config:
    addonsv1: {{toJson .config}}
`

const (
	packageOperatorName        = "packageOperatorReconciler"
	DeadMansSnitchUrlConfigKey = "deadMansSnitchUrl"
	PagerDutyKeyConfigKey      = "pagerDutyKey"
)

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
	addonDestNamespace := extractDestinationNamespace(addon)

	if addonDestNamespace == "" {
		return fmt.Errorf("no destination namespace configured in addon %s", addon.Name)
	}

	templateString := fmt.Sprintf(pkgTemplate,
		pkov1alpha1.GroupVersion,
		addon.Name,
		addon.Namespace,
		addon.Spec.AddonPackageOperator.Image,
	)

	pkg := &pkov1alpha1.ClusterObjectTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name,
			Namespace: addon.Namespace,
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
							Destination: "." + DeadMansSnitchUrlConfigKey,
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
							Destination: "." + PagerDutyKeyConfigKey,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(addon, &pkg.ObjectMeta, r.Scheme); err != nil {
		return fmt.Errorf("set owner reference: %w", err)
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

	r.updateAddonStatus(addon, existing)

	return nil
}

func extractDestinationNamespace(addon *addonsv1alpha1.Addon) string {
	switch addon.Spec.Install.Type {
	case addonsv1alpha1.OLMAllNamespaces:
		specNamespace := addon.Spec.Install.OLMAllNamespaces.Namespace
		if len(specNamespace) < 1 {
			return "openshift-operators"
		}
		return specNamespace
	case addonsv1alpha1.OLMOwnNamespace:
		return addon.Spec.Install.OLMOwnNamespace.Namespace
	default:
		return ""
	}
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

func (r *PackageOperatorReconciler) updateAddonStatus(addon *addonsv1alpha1.Addon, clusterObjectTemplate *pkov1alpha1.ClusterObjectTemplate) {
	availableCondition := meta.FindStatusCondition(clusterObjectTemplate.Status.Conditions, pkov1alpha1.PackageAvailable)
	if availableCondition == nil ||
		availableCondition.ObservedGeneration != clusterObjectTemplate.GetGeneration() ||
		availableCondition.Status != metav1.ConditionTrue {
		reportUnreadyClusterObjectTemplate(addon)
	}
}

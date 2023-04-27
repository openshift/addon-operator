package addon

import (
	"context"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"

	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
)

const PkoPkgTemplate = `
apiVersion: "%s"
kind: ClusterPackage
metadata:
  name: "%s"
spec:
  image: "%s"
  config:
    addonsv1: {{toJson (merge .config (dict "%s" "%s" "%s" "%s" "%s" "%s"))}}
`

const (
	packageOperatorName        = "packageOperatorReconciler"
	AddonParametersConfigKey   = "addonParameters"
	ClusterIDConfigKey         = "clusterID"
	DeadMansSnitchUrlConfigKey = "deadMansSnitchUrl"
	OcmClusterIDConfigKey      = "ocmClusterID"
	PagerDutyKeyConfigKey      = "pagerDutyKey"
	TargetNamespaceConfigKey   = "targetNamespace"
)

type PackageOperatorReconciler struct {
	Client       client.Client
	Scheme       *runtime.Scheme
	ClusterID    string
	OcmClusterID string
}

func (r *PackageOperatorReconciler) Name() string { return packageOperatorName }

func (r *PackageOperatorReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if addon.Spec.AddonPackageOperator == nil {
		return ctrl.Result{}, r.ensureClusterObjectTemplateTornDown(ctx, addon)
	}
	return r.reconcileClusterObjectTemplate(ctx, addon)
}

func (r *PackageOperatorReconciler) reconcileClusterObjectTemplate(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	addonDestNamespace := extractDestinationNamespace(addon)

	if len(addonDestNamespace) < 1 {
		return ctrl.Result{}, errors.New(fmt.Sprintf("no destination namespace configured in addon %s", addon.Name))
	}

	templateString := fmt.Sprintf(PkoPkgTemplate,
		pkov1alpha1.GroupVersion,
		addon.Name,
		addon.Spec.AddonPackageOperator.Image,
		ClusterIDConfigKey, r.ClusterID,
		OcmClusterIDConfigKey, r.OcmClusterID,
		TargetNamespaceConfigKey, addonDestNamespace,
	)

	clusterObjectTemplate := &pkov1alpha1.ClusterObjectTemplate{
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
					Name:       "addon-" + addon.Name + "-parameters",
					Namespace:  addonDestNamespace,
					Items: []pkov1alpha1.ObjectTemplateSourceItem{
						{
							Key:         ".data",
							Destination: "." + AddonParametersConfigKey,
						},
					},
				},
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

	if err := controllerutil.SetControllerReference(addon, clusterObjectTemplate, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting owner reference: %w", err)
	}

	existingClusterObjectTemplate, err := r.getExistingClusterObjectTemplate(ctx, addon)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if err := r.Client.Create(ctx, clusterObjectTemplate); err != nil {
				return ctrl.Result{}, fmt.Errorf("creating ClusterObjectTemplate object: %w", err)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting ClusterObjectTemplate object: %w", err)
	}

	clusterObjectTemplate.ResourceVersion = existingClusterObjectTemplate.ResourceVersion
	if err := r.Client.Update(ctx, clusterObjectTemplate); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating ClusterObjectTemplate object: %w", err)
	}
	r.updateAddonStatus(addon, existingClusterObjectTemplate)

	return ctrl.Result{}, nil
}

func (r *PackageOperatorReconciler) updateAddonStatus(addon *addonsv1alpha1.Addon, clusterObjectTemplate *pkov1alpha1.ClusterObjectTemplate) {
	availableCondition := meta.FindStatusCondition(clusterObjectTemplate.Status.Conditions, pkov1alpha1.PackageAvailable)
	if availableCondition == nil ||
		availableCondition.ObservedGeneration != clusterObjectTemplate.GetGeneration() ||
		availableCondition.Status != metav1.ConditionTrue {
		reportUnreadyClusterObjectTemplate(addon)
	}
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

func (r *PackageOperatorReconciler) ensureClusterObjectTemplateTornDown(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	existing, err := r.getExistingClusterObjectTemplate(ctx, addon)
	if err != nil {
		if k8serrors.IsNotFound(err) {
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

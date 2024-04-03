package addon

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"

	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
)

const PkoPkgTemplate = `
apiVersion: "%[1]s"
kind: ClusterPackage
metadata:
  name: "%[2]s"
spec:
  image: "%[3]s"
  config:
    addonsv1: {{toJson (
		merge
			(.config | b64decMap)
			(hasKey .config "%[4]s" | ternary (dict "%[4]s" (index .config "%[4]s" | b64decMap)) (dict))
			(hasKey .config "%[13]s" | ternary (dict "%[13]s" (index .config "%[13]s" | b64decMap)) (dict))
			(dict "%[5]s" "%[6]s" "%[7]s" "%[8]s" "%[9]s" "%[10]s" "%[11]s" "%[12]s")
	)}}
`

const (
	packageOperatorName        = "packageOperatorReconciler"
	ClusterIDConfigKey         = "clusterID"
	DeadMansSnitchUrlConfigKey = "deadMansSnitchUrl"
	OcmClusterIDConfigKey      = "ocmClusterID"
	OcmClusterNameConfigKey    = "ocmClusterName"
	PagerDutyKeyConfigKey      = "pagerDutyKey"
	ParametersConfigKey        = "parameters"
	TargetNamespaceConfigKey   = "targetNamespace"
	SendGridConfigKey          = "smtp"
)

type OcmClusterInfo struct {
	ID   string
	Name string
}

type (
	OcmClusterInfoGetter func() OcmClusterInfo
)

type PackageOperatorReconciler struct {
	Client         client.Client
	Scheme         *runtime.Scheme
	ClusterID      string
	OcmClusterInfo OcmClusterInfoGetter
	recorder       *metrics.Recorder
}

func (r *PackageOperatorReconciler) Name() string { return packageOperatorName }

func (r *PackageOperatorReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	reconErr := metrics.NewReconcileError("addon", r.recorder, true)
	if addon.Spec.AddonPackageOperator == nil {
		err := r.ensureClusterObjectTemplateTornDown(ctx, addon)
		if err != nil {
			err = reconErr.Join(err, controllers.ErrEnsureDeleteClusterObjectTemplate)
		}
		return ctrl.Result{}, err
	}

	return r.reconcileClusterObjectTemplate(ctx, addon)
}

func (r *PackageOperatorReconciler) reconcileClusterObjectTemplate(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	addonDestNamespace := extractDestinationNamespace(addon)
	reconErr := metrics.NewReconcileError("addon", r.recorder, true)

	if len(addonDestNamespace) < 1 {
		err := reconErr.Join(
			fmt.Errorf("no destination namespace configured in addon %s", addon.Name),
			controllers.ErrReconcileClusterObjectTemplate,
		)
		return ctrl.Result{}, err
	}



	
	ocmClusterInfo := r.OcmClusterInfo()

	templateString := fmt.Sprintf(PkoPkgTemplate,
		pkov1alpha1.GroupVersion,
		addon.Name,
		addon.Spec.AddonPackageOperator.Image,
		ParametersConfigKey,
		ClusterIDConfigKey, r.ClusterID,
		OcmClusterIDConfigKey, ocmClusterInfo.ID,
		OcmClusterNameConfigKey, ocmClusterInfo.Name,
		TargetNamespaceConfigKey, addonDestNamespace,
		SendGridConfigKey,
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
							Destination: "." + ParametersConfigKey,
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
				{
					Optional:   true,
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       addon.Name + "-smtp",
					Namespace:  addonDestNamespace,
					Items: []pkov1alpha1.ObjectTemplateSourceItem{
						{
							Key:         ".data",
							Destination: "." + SendGridConfigKey,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(addon, clusterObjectTemplate, r.Scheme); err != nil {
		newErr := reconErr.Join(
			fmt.Errorf("setting owner reference: %w", err),
			controllers.ErrReconcileClusterObjectTemplate,
		)
		return ctrl.Result{}, newErr
	}

	existingClusterObjectTemplate, err := r.getExistingClusterObjectTemplate(ctx, addon)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if err := r.Client.Create(ctx, clusterObjectTemplate); err != nil {
				newErr := reconErr.Join(
					fmt.Errorf("creating ClusterObjectTemplate object: %w", err),
					controllers.ErrReconcileClusterObjectTemplate,
				)

				return ctrl.Result{}, newErr
			}
			return ctrl.Result{}, nil
		}
		newErr := reconErr.Join(
			fmt.Errorf("getting ClusterObjectTemplate object: %w", err),
			controllers.ErrReconcileClusterObjectTemplate,
		)
		return ctrl.Result{}, newErr
	}
	ownedByAdo := controllers.HasSameController(existingClusterObjectTemplate, clusterObjectTemplate)
	specChanged := !equality.Semantic.DeepEqual(existingClusterObjectTemplate.Spec, clusterObjectTemplate.Spec)
	if specChanged || !ownedByAdo {
		existingClusterObjectTemplate.Spec = clusterObjectTemplate.Spec
		existingClusterObjectTemplate.OwnerReferences = clusterObjectTemplate.OwnerReferences
		if err := r.Client.Update(ctx, existingClusterObjectTemplate); err != nil {
			newErr := reconErr.Join(
				fmt.Errorf("updating ClusterObjectTemplate object: %w", err),
				controllers.ErrReconcileClusterObjectTemplate,
			)
			return ctrl.Result{}, newErr
		}
		return ctrl.Result{}, nil
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

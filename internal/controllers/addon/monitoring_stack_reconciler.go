package addon

import (
	"context"
	"fmt"
	"strings"

	monv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"
	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

const MONITORING_STACK_RECONCILER_NAME = "monitoringStackReconciler"

type monitoringStackReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func (r monitoringStackReconciler) Name() string {
	return MONITORING_STACK_RECONCILER_NAME
}

func (r *monitoringStackReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (ctrl.Result, error) {

	if addon.Spec.Monitoring == nil {
		return reconcile.Result{}, nil
	}

	if addon.Spec.Monitoring.MonitoringStack == nil {
		return reconcile.Result{}, nil
	}

	// ensure creation of MonitoringStack object
	if err := r.ensureMonitoringStack(ctx, addon); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *monitoringStackReconciler) ensureMonitoringStack(ctx context.Context,
	addon *addonsv1alpha1.Addon) error {

	if !HasMonitoringStack(addon) {
		return nil
	}

	// create desired MonitoringStack
	desiredMonitoringStack, err := r.getDesiredMonitoringStack(ctx, addon)
	if err != nil {
		return err
	}

	// returns observed MonitoringStack object
	_, err = r.reconcileMonitoringStack(ctx, desiredMonitoringStack)

	// TODO: Read the Status of the observed MonitoringStack and:
	// 1. Report it to Addon CR Status
	// 2. Expose corresponding metrics
	return err
}

// helper function to generate desired MonitoringStack object
func (r *monitoringStackReconciler) getDesiredMonitoringStack(ctx context.Context,
	addon *addonsv1alpha1.Addon) (*obov1alpha1.MonitoringStack, error) {

	commonConfig, stop := parseAddonInstallConfig(controllers.LoggerFromContext(ctx), addon)
	if stop {
		return nil, fmt.Errorf("error parsing Addon config")
	}

	var (
		remoteWriteURL     string
		oauthConfig        *monv1.OAuth2
		writeRelabelConfig []monv1.RelabelConfig
	)

	rhobsRemoteWriteConfig := addon.Spec.Monitoring.MonitoringStack.RHOBSRemoteWriteConfig.DeepCopy()
	if rhobsRemoteWriteConfig != nil {
		remoteWriteURL = rhobsRemoteWriteConfig.URL
		oauthConfig = rhobsRemoteWriteConfig.OAuth2.DeepCopy()
		writeRelabelConfig = getWriteRelabelConfigFromAllowlist(rhobsRemoteWriteConfig.Allowlist)
	}

	desiredMonitoringStack := &obov1alpha1.MonitoringStack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getMonitoringStackName(addon.Name),
			Namespace: commonConfig.Namespace,
		},
		Spec: obov1alpha1.MonitoringStackSpec{
			Retention: "30d",
			ResourceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					controllers.MSOLabel: addon.Name,
				},
			},
			PrometheusConfig: &obov1alpha1.PrometheusConfig{
				RemoteWrite: []monv1.RemoteWriteSpec{
					{
						URL:                 remoteWriteURL,
						OAuth2:              oauthConfig,
						WriteRelabelConfigs: writeRelabelConfig,
					},
				},
			},
		},
	}

	// add common labels and owner references
	controllers.AddCommonLabels(desiredMonitoringStack, addon)
	if err := controllerutil.SetControllerReference(addon, desiredMonitoringStack,
		r.scheme); err != nil {
		return nil, err
	}
	return desiredMonitoringStack, nil
}

func getServiceMonitorName(addonName string) string {
	return fmt.Sprintf("%s-service-monitor", addonName)
}

func getMonitoringStackName(addonName string) string {
	return fmt.Sprintf("%s-monitoring-stack", addonName)
}

func (r *monitoringStackReconciler) reconcileMonitoringStack(ctx context.Context,
	desiredMonitoringStack *obov1alpha1.MonitoringStack) (*obov1alpha1.MonitoringStack, error) {

	// get existing MonitoringStack
	currentMonitoringStack := &obov1alpha1.MonitoringStack{}
	if err := r.client.Get(ctx, client.ObjectKey{
		Name:      desiredMonitoringStack.Name,
		Namespace: desiredMonitoringStack.Namespace,
	}, currentMonitoringStack); err != nil {
		// create desired MonitoringStack if it does not exist
		if k8sApiErrors.IsNotFound(err) {
			return desiredMonitoringStack, r.client.Create(ctx, desiredMonitoringStack)
		}
		return nil, err
	}

	// only update when spec or ownerReference has changed
	var (
		ownedByAddon  = controllers.HasSameController(currentMonitoringStack, desiredMonitoringStack)
		specChanged   = !equality.Semantic.DeepEqual(desiredMonitoringStack.Spec, currentMonitoringStack.Spec)
		currentLabels = labels.Set(currentMonitoringStack.Labels)
		newLabels     = labels.Merge(currentLabels, labels.Set(desiredMonitoringStack.Labels))
	)

	if specChanged || !ownedByAddon || !labels.Equals(newLabels, currentLabels) {
		// copy new spec into existing object and update in the k8s api
		currentMonitoringStack.Spec = desiredMonitoringStack.Spec
		currentMonitoringStack.OwnerReferences = desiredMonitoringStack.OwnerReferences
		currentMonitoringStack.Labels = newLabels
		return currentMonitoringStack, r.client.Update(ctx, currentMonitoringStack)
	}
	return currentMonitoringStack, nil
}

func getWriteRelabelConfigFromAllowlist(allowlist []string) []monv1.RelabelConfig {
	relabelConfigs := []monv1.RelabelConfig{}
	if len(allowlist) == 0 {
		return relabelConfigs
	}

	regex := fmt.Sprintf("(%s)", strings.Join(allowlist[:], "|"))
	relabelConfig := monv1.RelabelConfig{
		Action:       "keep",
		SourceLabels: []monv1.LabelName{"[__name__]"},
		Regex:        regex,
	}
	relabelConfigs = append(relabelConfigs, relabelConfig)
	return relabelConfigs
}

package addon

import (
	"context"
	"errors"
	"fmt"
	"strings"

	monv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"
	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"
)

const MONITORING_STACK_RECONCILER_NAME = "monitoringStackReconciler"

var errMonitoringStackSpecNotFound = fmt.Errorf("monitoring stack spec not found")

type monitoringStackReconciler struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder *metrics.Recorder
}

func (r *monitoringStackReconciler) Name() string {
	return MONITORING_STACK_RECONCILER_NAME
}

func (r *monitoringStackReconciler) Order() subReconcilerOrder {
	return MonitoringStackReconcilerOrder
}

func (r *monitoringStackReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	reconErr := metrics.NewReconcileError("addon", r.recorder, true)

	// ensure creation of MonitoringStack object
	latestMonitoringStack, err := r.ensureMonitoringStack(ctx, addon)
	if err != nil {
		if errors.Is(err, errMonitoringStackSpecNotFound) {
			return resultNil, nil
		}

		err = reconErr.Join(err, controllers.ErrEnsureCreateMonitoringStack)
		return resultNil, err
	}

	// propagate the recently reconciled (created/updated) monitoring stack's status to the owner Addon
	if monitoringStackAvailable := r.propagateMonitoringStackStatusToAddon(latestMonitoringStack, addon); !monitoringStackAvailable {
		return resultRequeue, nil
	}

	return resultNil, nil
}

func (r *monitoringStackReconciler) ensureMonitoringStack(ctx context.Context,
	addon *addonsv1alpha1.Addon) (*obov1alpha1.MonitoringStack, error) {
	if !HasMonitoringStack(addon) {
		return nil, errMonitoringStackSpecNotFound
	}
	// create desired MonitoringStack
	desiredMonitoringStack, err := r.getDesiredMonitoringStack(ctx, addon)
	if err != nil {
		return nil, err
	}

	// returns observed MonitoringStack object
	reconciledMonitoringStack, err := r.reconcileMonitoringStack(ctx, desiredMonitoringStack)
	if err != nil {
		return nil, err
	}
	return reconciledMonitoringStack, nil
}

func (r *monitoringStackReconciler) propagateMonitoringStackStatusToAddon(monitoringStack *obov1alpha1.MonitoringStack, addon *addonsv1alpha1.Addon) (monitoringStackStackAvailable bool) {
	availableCondition, reconciledCondition := obov1alpha1.Condition{}, obov1alpha1.Condition{}
	availableConditionFound, reconciledConditionFound := false, false

	// iterate until all conditions have been traversed or both `available` and `reconcile` conditions have been found
	for i, cond := range monitoringStack.Status.Conditions {
		if availableConditionFound && reconciledConditionFound {
			break
		}

		switch cond.Type {
		case obov1alpha1.AvailableCondition:
			availableCondition = monitoringStack.Status.Conditions[i]
			availableConditionFound = true
		case obov1alpha1.ReconciledCondition:
			reconciledCondition = monitoringStack.Status.Conditions[i]
			reconciledConditionFound = true
		}
	}

	if availableConditionFound {
		if availableCondition.Status == obov1alpha1.ConditionTrue {
			return true
		}
		reportUnreadyMonitoringStack(addon, fmt.Sprintf("MonitoringStack Unavailable: %s", availableCondition.Message))
		return false
	}

	if reconciledConditionFound {
		if reconciledCondition.Status == obov1alpha1.ConditionTrue {
			reportUnreadyMonitoringStack(addon, "MonitoringStack successfully reconciled: Pending MonitoringStack to be Available")
		} else {
			reportUnreadyMonitoringStack(addon, fmt.Sprintf("MonitoringStack failed to reconcile: %s", reconciledCondition.Message))
		}
		return false
	}

	reportUnreadyMonitoringStack(addon, "MonitoringStack pending to get reconciled")
	return false
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

	rhobsRemoteWriteConfig := addon.Spec.Monitoring.MonitoringStack.RHOBSRemoteWriteConfig
	if rhobsRemoteWriteConfig != nil {
		remoteWriteURL = rhobsRemoteWriteConfig.URL
		oauthConfig = rhobsRemoteWriteConfig.OAuth2
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

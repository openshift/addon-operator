package addon

import (
	"context"
	"errors"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
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

const MONITORING_FEDERATION_RECONCILER_NAME = "monitoringFederationReconciler"

type monitoringFederationReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func (r *monitoringFederationReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	log := controllers.LoggerFromContext(ctx)

	// Possibly ensure monitoring federation
	// Normally this would be configured before the addon workload is installed
	// but currently the addon workload creates the monitoring stack by itself
	// thus we want to create the service monitor as late as possible to ensure that
	// cluster-monitoring prom does not try to scrape a non-existent addon prometheus.

	result, err := r.ensureMonitoringFederation(ctx, addon)
	if errors.Is(err, controllers.ErrNotOwnedByUs) {
		log.Info("stopping", "reason", "monitoring federation namespace or serviceMonitor owned by something else")

		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure ServiceMonitor: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	// Remove possibly unwanted monitoring federation
	if err := r.ensureDeletionOfUnwantedMonitoringFederation(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure deletion of unwanted ServiceMonitors: %w", err)
	}
	return reconcile.Result{}, nil
}

func (r *monitoringFederationReconciler) Name() string {
	return MONITORING_FEDERATION_RECONCILER_NAME
}

// ensureMonitoringFederation inspects an addon's MonitoringFederation specification
// and if it exists ensures that a ServiceMonitor is present in the desired monitoring
// namespace.
func (r *monitoringFederationReconciler) ensureMonitoringFederation(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if !HasMonitoringFederation(addon) {
		return ctrl.Result{}, nil
	}

	result, err := r.ensureMonitoringNamespace(ctx, addon)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("ensuring monitoring Namespace: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}

	if err := r.ensureServiceMonitor(ctx, addon); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensuring ServiceMonitor: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *monitoringFederationReconciler) ensureMonitoringNamespace(
	ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	desired, err := r.desiredMonitoringNamespace(addon)
	if err != nil {
		return ctrl.Result{}, err
	}

	actual, err := r.actualMonitoringNamespace(ctx, addon)
	if k8sApiErrors.IsNotFound(err) {
		return ctrl.Result{}, r.client.Create(ctx, desired)
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting monitoring namespace: %w", err)
	}

	ownedByAddon := controllers.HasSameController(actual, desired)
	labelsChanged := !equality.Semantic.DeepEqual(actual.Labels, desired.Labels)

	if ownedByAddon && !labelsChanged {
		return ctrl.Result{}, nil
	}

	actual.OwnerReferences, actual.Labels = desired.OwnerReferences, desired.Labels

	if err := r.client.Update(ctx, actual); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating monitoring namespace: %w", err)
	}

	if actual.Status.Phase == corev1.NamespaceActive {
		return ctrl.Result{}, nil
	}

	reportUnreadyMonitoring(addon, fmt.Sprintf("namespace %q is not active", actual.Name))

	// Previously this would trigger exit and move on to the next phase.
	// However, given that the reconciliation is not complete an error should
	// be returned to requeue the work.
	return ctrl.Result{RequeueAfter: defaultRetryAfterTime}, nil
}

func (r *monitoringFederationReconciler) desiredMonitoringNamespace(addon *addonsv1alpha1.Addon) (*corev1.Namespace, error) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetMonitoringNamespaceName(addon),
			Labels: map[string]string{
				"openshift.io/cluster-monitoring": "true",
			},
		},
	}

	controllers.AddCommonLabels(namespace, addon)

	if err := controllerutil.SetControllerReference(addon, namespace, r.scheme); err != nil {
		return nil, err
	}

	return namespace, nil
}

func (r *monitoringFederationReconciler) actualMonitoringNamespace(
	ctx context.Context, addon *addonsv1alpha1.Addon) (*corev1.Namespace, error) {
	key := client.ObjectKey{
		Name: GetMonitoringNamespaceName(addon),
	}

	namespace := &corev1.Namespace{}
	if err := r.client.Get(ctx, key, namespace); err != nil {
		return nil, err
	}

	return namespace, nil
}

func (r *monitoringFederationReconciler) ensureServiceMonitor(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	desired, err := r.desiredServiceMonitor(addon)
	if err != nil {
		return err
	}

	actual, err := r.actualServiceMonitor(ctx, addon)
	if k8sApiErrors.IsNotFound(err) {
		return r.client.Create(ctx, desired)
	} else if err != nil {
		return fmt.Errorf("getting ServiceMonitor: %w", err)
	}

	currentLabels := labels.Set(actual.Labels)
	newLabels := labels.Merge(currentLabels, labels.Set(desired.Labels))
	ownedByAddon := controllers.HasSameController(actual, desired)
	specChanged := !equality.Semantic.DeepEqual(actual.Spec, desired.Spec)
	labelsChanged := !labels.Equals(currentLabels, newLabels)

	if ownedByAddon && !specChanged && !labelsChanged {
		return nil
	}

	actual.Spec = desired.Spec
	actual.Labels = newLabels
	actual.OwnerReferences = desired.OwnerReferences

	return r.client.Update(ctx, actual)
}

func (r *monitoringFederationReconciler) desiredServiceMonitor(addon *addonsv1alpha1.Addon) (*monitoringv1.ServiceMonitor, error) {
	serviceMonitor := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetMonitoringFederationServiceMonitorName(addon),
			Namespace: GetMonitoringNamespaceName(addon),
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: GetMonitoringFederationServiceMonitorEndpoints(addon),
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{addon.Spec.Monitoring.Federation.Namespace},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: addon.Spec.Monitoring.Federation.MatchLabels,
			},
		},
	}

	controllers.AddCommonLabels(serviceMonitor, addon)

	if err := controllerutil.SetControllerReference(addon, serviceMonitor, r.scheme); err != nil {
		return nil, fmt.Errorf("setting controller reference on ServiceMonitor: %w", err)
	}

	return serviceMonitor, nil
}

func (r *monitoringFederationReconciler) actualServiceMonitor(
	ctx context.Context, addon *addonsv1alpha1.Addon) (*monitoringv1.ServiceMonitor, error) {
	key := client.ObjectKey{
		Name:      GetMonitoringFederationServiceMonitorName(addon),
		Namespace: GetMonitoringNamespaceName(addon),
	}

	serviceMonitor := &monitoringv1.ServiceMonitor{}
	if err := r.client.Get(ctx, key, serviceMonitor); err != nil {
		return nil, err
	}

	return serviceMonitor, nil
}

// Ensure cleanup of ServiceMonitors that are not needed anymore for the given Addon resource
func (r *monitoringFederationReconciler) ensureDeletionOfUnwantedMonitoringFederation(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
) error {
	currentServiceMonitors, err := r.getOwnedServiceMonitorsViaCommonLabels(ctx, r.client, addon)
	if err != nil {
		return err
	}

	// A ServiceMonitor is wanted only if .spec.monitoring.federation is set
	wantedServiceMonitorName := ""
	if addon.Spec.Monitoring != nil && addon.Spec.Monitoring.Federation != nil {
		wantedServiceMonitorName = GetMonitoringFederationServiceMonitorName(addon)
	}

	for _, serviceMonitor := range currentServiceMonitors {
		if serviceMonitor.Name == wantedServiceMonitorName ||
			serviceMonitor.Name == getServiceMonitorName(addon.Name) {
			// don't delete
			continue
		}

		if err := client.IgnoreNotFound(r.client.Delete(ctx, serviceMonitor)); err != nil {
			return fmt.Errorf("could not remove monitoring federation ServiceMonitor: %w", err)
		}
	}

	if wantedServiceMonitorName == "" {
		err := ensureNamespaceDeletion(ctx, r.client, GetMonitoringNamespaceName(addon))
		if err != nil {
			return fmt.Errorf("could not remove monitoring federation Namespace: %w", err)
		}
	}

	return nil
}

// Get all ServiceMonitors that have common labels matching the given Addon resource
func (r *monitoringFederationReconciler) getOwnedServiceMonitorsViaCommonLabels(
	ctx context.Context,
	c client.Client,
	addon *addonsv1alpha1.Addon) ([]*monitoringv1.ServiceMonitor, error) {
	selector := controllers.CommonLabelsAsLabelSelector(addon)

	list := &monitoringv1.ServiceMonitorList{}
	if err := c.List(ctx, list, &client.ListOptions{
		LabelSelector: client.MatchingLabelsSelector{
			Selector: selector,
		},
	}); err != nil {
		return nil, fmt.Errorf("could not list owned ServiceMonitors")
	}

	return list.Items, nil
}

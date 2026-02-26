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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"
)

const MONITORING_FEDERATION_RECONCILER_NAME = "monitoringFederationReconciler"

type monitoringFederationReconciler struct {
	client, uncachedClient client.Client
	scheme                 *runtime.Scheme
	addonOperatorNamespace string
	recorder               *metrics.Recorder
}

func (r *monitoringFederationReconciler) Reconcile(ctx context.Context,
	addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	log := controllers.LoggerFromContext(ctx)
	reconErr := metrics.NewReconcileError("addon", r.recorder, true)
	// Possibly ensure monitoring federation
	// Normally this would be configured before the addon workload is installed
	// but currently the addon workload creates the monitoring stack by itself
	// thus we want to create the service monitor as late as possible to ensure that
	// cluster-monitoring prom does not try to scrape a non-existent addon prometheus.

	res, err := r.ensureMonitoringFederation(ctx, addon)
	if errors.Is(err, controllers.ErrNotOwnedByUs) {
		log.Info("stopping", "reason", "monitoring federation namespace or serviceMonitor owned by something else")

		return resultNil, nil
	} else if err != nil {
		err = reconErr.Join(err, controllers.ErrEnsureCreateServiceMonitor)
		return resultNil, err
	} else if !res.IsZero() {
		return res, nil
	}

	// Remove possibly unwanted monitoring federation
	if err := r.ensureDeletionOfUnwantedMonitoringFederation(ctx, addon); err != nil {
		err = reconErr.Join(
			err,
			controllers.ErrEnsureDeleteServiceMonitor,
		)
		return resultNil, err
	}
	return resultNil, nil
}

func (r *monitoringFederationReconciler) Name() string {
	return MONITORING_FEDERATION_RECONCILER_NAME
}

func (r *monitoringFederationReconciler) Order() subReconcilerOrder {
	return MonitoringFederationReconcilerOrder
}

// ensureMonitoringFederation inspects an addon's MonitoringFederation specification
// and if it exists ensures that a ServiceMonitor is present in the desired monitoring
// namespace.
func (r *monitoringFederationReconciler) ensureMonitoringFederation(ctx context.Context, addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	if !HasMonitoringFederation(addon) {
		return resultNil, nil
	}

	result, err := r.ensureMonitoringNamespace(ctx, addon)
	if err != nil {
		return resultNil, fmt.Errorf("ensuring monitoring Namespace: %w", err)
	} else if !result.IsZero() {
		return result, nil
	}
	sresult, serr := r.reconcileServiceMonitor(ctx, addon)
	if serr != nil {
		return resultNil, fmt.Errorf("ensuring ServiceMonitor: %w", err)
	}
	if !sresult.IsZero() {
		return sresult, nil
	}

	return resultNil, nil
}

func (r *monitoringFederationReconciler) ensureMonitoringNamespace(
	ctx context.Context, addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	desired, err := r.desiredMonitoringNamespace(addon)
	if err != nil {
		return resultNil, err
	}

	actual, err := r.actualMonitoringNamespace(ctx, addon)
	if k8sApiErrors.IsNotFound(err) {
		return resultNil, r.client.Create(ctx, desired)
	} else if err != nil {
		return resultNil, fmt.Errorf("getting monitoring namespace: %w", err)
	}

	ownedByAddon := controllers.HasSameController(actual, desired)
	labelsChanged := !equality.Semantic.DeepEqual(actual.Labels, desired.Labels)

	if ownedByAddon && !labelsChanged {
		return resultNil, nil
	}

	actual.OwnerReferences, actual.Labels = desired.OwnerReferences, desired.Labels

	if err := r.client.Update(ctx, actual); err != nil {
		return resultNil, fmt.Errorf("updating monitoring namespace: %w", err)
	}

	if actual.Status.Phase == corev1.NamespaceActive {
		return resultNil, nil
	}

	reportUnreadyMonitoringFederation(addon, fmt.Sprintf("namespace %q is not active", actual.Name))

	// Previously this would trigger exit and move on to the next phase.
	// However, given that the reconciliation is not complete an error should
	// be returned to requeue the work.
	return resultRequeueAfter(defaultRetryAfterTime), nil
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

func (r *monitoringFederationReconciler) reconcileServiceMonitor(ctx context.Context, addon *addonsv1alpha1.Addon) (subReconcilerResult, error) {
	bearerTokenReconcileRes, tokenSecret, err := r.reconcileBearerTokenSecretForAddon(ctx, addon)
	if err != nil {
		return resultNil, err
	}
	if !bearerTokenReconcileRes.IsZero() {
		return bearerTokenReconcileRes, nil
	}

	desired, err := r.desiredServiceMonitor(addon, tokenSecret)
	if err != nil {
		return resultNil, err
	}

	actual, err := r.actualServiceMonitor(ctx, addon)
	if k8sApiErrors.IsNotFound(err) {
		return resultNil, r.client.Create(ctx, desired)
	} else if err != nil {
		return resultNil, fmt.Errorf("getting ServiceMonitor: %w", err)
	}

	currentLabels := labels.Set(actual.Labels)
	newLabels := labels.Merge(currentLabels, labels.Set(desired.Labels))
	ownedByAddon := controllers.HasSameController(actual, desired)
	specChanged := !equality.Semantic.DeepEqual(actual.Spec, desired.Spec)
	labelsChanged := !labels.Equals(currentLabels, newLabels)

	if ownedByAddon && !specChanged && !labelsChanged {
		return resultNil, nil
	}

	actual.Spec = desired.Spec
	actual.Labels = newLabels
	actual.OwnerReferences = desired.OwnerReferences

	return resultNil, r.client.Update(ctx, actual)
}

func (r *monitoringFederationReconciler) desiredServiceMonitor(addon *addonsv1alpha1.Addon, bearerTokenSecret *corev1.Secret) (*monitoringv1.ServiceMonitor, error) {
	serviceMonitor := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetMonitoringFederationServiceMonitorName(addon),
			Namespace: GetMonitoringNamespaceName(addon),
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: GetMonitoringFederationServiceMonitorEndpoints(addon, bearerTokenSecret),
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

func (r *monitoringFederationReconciler) reconcileBearerTokenSecretForAddon(ctx context.Context, addon *addonsv1alpha1.Addon) (subReconcilerResult, *corev1.Secret, error) {
	key := types.NamespacedName{
		Name:      "addon-operator-prom-token",
		Namespace: r.addonOperatorNamespace,
	}

	log := controllers.LoggerFromContext(ctx)
	log.Info("finding token secret openshift-addon-operator")
	addonOperatorPromTokenSecret := &corev1.Secret{}
	// check if ADO ns has the SA token required for the SM
	if err := r.uncachedClient.Get(ctx, key, addonOperatorPromTokenSecret); err != nil {
		return resultNil, nil, fmt.Errorf("addonOperator namespace does not have the prom token secret: %w", err)
	}

	desiredBearertokensecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-bearertoken-secret", addon.Name),
			Namespace: GetMonitoringNamespaceName(addon),
		},
		Data: addonOperatorPromTokenSecret.Data,
	}

	if err := controllerutil.SetControllerReference(addon, desiredBearertokensecret, r.scheme); err != nil {
		return resultNil, nil, fmt.Errorf("setting owner reference: %w", err)
	}

	existingBearerTokenSecret := &corev1.Secret{}
	if err := r.uncachedClient.Get(
		ctx,
		types.NamespacedName{Name: desiredBearertokensecret.Name, Namespace: desiredBearertokensecret.Namespace},
		existingBearerTokenSecret,
	); err != nil {
		if k8sApiErrors.IsNotFound(err) {
			log.Info("creating the sa secret in the monitoring namespace")
			return resultRequeueAfter(defaultRetryAfterTime), nil, r.client.Create(ctx, desiredBearertokensecret)
		}
		return resultNil, nil, err
	}

	ownedByAddon := controllers.HasSameController(desiredBearertokensecret, existingBearerTokenSecret)
	specChanged := !equality.Semantic.DeepEqual(existingBearerTokenSecret.Data, desiredBearertokensecret.Data)
	currentLabels := labels.Set(existingBearerTokenSecret.Labels)
	newLabels := labels.Merge(currentLabels, labels.Set(desiredBearertokensecret.Labels))
	if specChanged || !ownedByAddon || !labels.Equals(newLabels, currentLabels) {
		existingBearerTokenSecret.Data = desiredBearertokensecret.Data
		existingBearerTokenSecret.OwnerReferences = desiredBearertokensecret.OwnerReferences
		existingBearerTokenSecret.Labels = newLabels
		log.Info("updating the sa secret in the monitoring namespace")
		return resultRequeueAfter(defaultRetryAfterTime), nil, r.client.Update(ctx, existingBearerTokenSecret)
	}
	log.Info("Already present Secret")
	return resultNil, existingBearerTokenSecret, nil
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
		if serviceMonitor.Name == wantedServiceMonitorName {
			// don't delete
			continue
		}

		if err := client.IgnoreNotFound(r.client.Delete(ctx, &serviceMonitor)); err != nil {
			return fmt.Errorf("could not remove monitoring federation ServiceMonitor: %w", err)
		}
		if err := client.IgnoreNotFound(r.client.Delete(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-bearertoken-secret", addon.Name),
				Namespace: GetMonitoringNamespaceName(addon),
			}})); err != nil {
			return fmt.Errorf("could not remove monitoring federation ServiceMonitor Secret in the SM ns for the Addon : %w", err)
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
	addon *addonsv1alpha1.Addon) ([]monitoringv1.ServiceMonitor, error) {
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

package addon

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
)

// Ensure cleanup of ServiceMonitors that are not needed anymore for the given Addon resource
func (r *AddonReconciler) ensureDeletionOfUnwantedMonitoringFederation(
	ctx context.Context,
	addon *addonsv1alpha1.Addon,
) error {
	currentServiceMonitors, err := r.getOwnedServiceMonitorsViaCommonLabels(ctx, r.Client, addon)
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

		if err := client.IgnoreNotFound(r.Client.Delete(ctx, serviceMonitor)); err != nil {
			return fmt.Errorf("could not remove monitoring federation ServiceMonitor: %w", err)
		}
	}

	if wantedServiceMonitorName == "" {
		err := ensureNamespaceDeletion(ctx, r.Client, GetMonitoringNamespaceName(addon))
		if err != nil {
			return fmt.Errorf("could not remove monitoring federation Namespace: %w", err)
		}
	}

	return nil
}

// Get all ServiceMonitors that have common labels matching the given Addon resource
func (r *AddonReconciler) getOwnedServiceMonitorsViaCommonLabels(
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

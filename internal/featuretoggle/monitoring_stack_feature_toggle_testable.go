package featuretoggle

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"

	"github.com/mt-sre/devkube/dev"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
)

var observabilityOperatorVersion = "0.0.15"

func (m *MonitoringStackFeatureToggle) Enable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureFlags: m.GetFeatureToggleIdentifier(),
				},
			}
			if err := m.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already enabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureFlags, ",")
	isMonitoringStackAlreadyEnabled := stringPresentInSlice(m.GetFeatureToggleIdentifier(), existingFeatureToggles)
	if isMonitoringStackAlreadyEnabled {
		return nil
	}
	if adoInCluster.Spec.FeatureFlags == "" {
		adoInCluster.Spec.FeatureFlags = m.GetFeatureToggleIdentifier()
	} else {
		adoInCluster.Spec.FeatureFlags += "," + m.GetFeatureToggleIdentifier()
	}
	if err := m.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func (m *MonitoringStackFeatureToggle) Disable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureFlags: "",
				},
			}
			if err := m.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already disabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureFlags, ",")
	isMonitoringStackAlreadyEnabled := stringPresentInSlice(m.GetFeatureToggleIdentifier(), existingFeatureToggles)
	if !isMonitoringStackAlreadyEnabled {
		return nil
	}
	updatedFeatureToggles := ""
	for _, featTog := range existingFeatureToggles {
		if featTog == m.GetFeatureToggleIdentifier() {
			continue
		}
		updatedFeatureToggles += "," + featTog
	}
	if len(updatedFeatureToggles) != 0 {
		updatedFeatureToggles = updatedFeatureToggles[1:]
	}
	adoInCluster.Spec.FeatureFlags = updatedFeatureToggles
	if err := m.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func (m *MonitoringStackFeatureToggle) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

func renderObservabilityOperatorCatalogSource(ctx context.Context, cluster *dev.Cluster) (*operatorsv1alpha1.CatalogSource, error) {
	objs, err := dev.LoadKubernetesObjectsFromFile("config/deploy/observability-operator/catalog-source.yaml.tpl")
	if err != nil {
		return nil, fmt.Errorf("failed to load the prometheus-remote-storage-mock deployment.yaml.tpl: %w", err)
	}

	// Replace version
	observabilityOperatorCatalogSource := &operatorsv1alpha1.CatalogSource{}
	if err := cluster.Scheme.Convert(&objs[0], observabilityOperatorCatalogSource, ctx); err != nil {
		return nil, fmt.Errorf("failed to convert the catalog source: %w", err)
	}

	observabilityOperatorCatalogSourceImage := fmt.Sprintf("quay.io/rhobs/observability-operator-catalog:%s", observabilityOperatorVersion)
	observabilityOperatorCatalogSource.Spec.Image = observabilityOperatorCatalogSourceImage

	return observabilityOperatorCatalogSource, nil
}

func (m *MonitoringStackFeatureToggle) PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error {
	observabilityOperatorCatalogSource, err := renderObservabilityOperatorCatalogSource(ctx, clusterCreated)
	if err != nil {
		return fmt.Errorf("failed to render the observability operator catalog source from its template: %w", err)
	}

	if err := clusterCreated.CreateAndWaitFromFiles(ctx, []string{
		"config/deploy/observability-operator/namespace.yaml",
	}); err != nil {
		return fmt.Errorf("failed to load the namespace for observability-operator: %w", err)
	}

	if err := clusterCreated.CreateAndWaitForReadiness(ctx, observabilityOperatorCatalogSource); err != nil {
		return fmt.Errorf("failed to load the catalog source for observability-operator: %w", err)
	}

	if err := clusterCreated.CreateAndWaitFromFiles(ctx, []string{
		"config/deploy/observability-operator/operator-group.yaml",
		"config/deploy/observability-operator/subscription.yaml",
	}); err != nil {
		return fmt.Errorf("failed to load the operator-group/subscription for observability-operator: %w", err)
	}
	return nil
}

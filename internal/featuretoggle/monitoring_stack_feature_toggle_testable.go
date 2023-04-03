package featuretoggle

import (
	"context"
	"fmt"

	"github.com/mt-sre/devkube/dev"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
)

var observabilityOperatorVersion = "0.0.15"

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

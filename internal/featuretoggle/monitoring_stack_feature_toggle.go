package featuretoggle

import (
	"context"
	"fmt"

	"github.com/mt-sre/devkube/dev"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

var _ Handler = (*MonitoringStackFeatureToggle)(nil)

const observabilityOperatorVersion = "0.0.15"

type MonitoringStackFeatureToggle struct {
	Handler
	Client                      client.Client
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (m *MonitoringStackFeatureToggle) Name() string {
	return "Monitoring Stack Reconciliation Feature Toggle"
}

func (m *MonitoringStackFeatureToggle) GetFeatureToggleIdentifier() string {
	return "EXPERIMENTAL_FEATURES"
}

func (m *MonitoringStackFeatureToggle) PreManagerSetupHandle(ctx context.Context) error {
	// nothing to handle before the manager is setup
	_ = obov1alpha1.AddToScheme(m.SchemeToUpdate)
	return nil
}

func (m *MonitoringStackFeatureToggle) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error {
	// use the manager's cached client and scheme to setup the monitoringStackReconcilerOpt addonReconcilerOpts w.r.t this featureToggleHandler
	*m.AddonReconcilerOptsToUpdate = append(*m.AddonReconcilerOptsToUpdate, addoncontroller.WithMonitoringStackReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	})
	return nil
}

// Used For Testing
func (m *MonitoringStackFeatureToggle) Enable(ctx context.Context) error {
	return EnableFeatureToggle(ctx, m.Client, m.GetFeatureToggleIdentifier())
}

// Used For Testing
func (m *MonitoringStackFeatureToggle) Disable(ctx context.Context) error {
	return DisableFeatureToggle(ctx, m.Client, m.GetFeatureToggleIdentifier())
}

// Used For Testing
func (m *MonitoringStackFeatureToggle) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

// Used For Testing
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

// Used For Testing
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

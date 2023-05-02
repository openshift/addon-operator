package featureflag

import (
	"context"
	"fmt"

	"github.com/mt-sre/devkube/dev"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

const observabilityOperatorVersion = "0.0.15"

var _ Handler = (*MonitoringStackFeatureFlag)(nil)

type MonitoringStackFeatureFlag struct {
	Handler
	Client                      client.Client
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (m *MonitoringStackFeatureFlag) Name() string {
	return "Monitoring Stack Reconciliation Feature Flag"
}

func (m *MonitoringStackFeatureFlag) GetFeatureFlagIdentifier() string {
	// TODO: should this be changed to "MONITORING_STACK"?
	return "EXPERIMENTAL_FEATURES"
}

// PreManagerSetupHandle sets up to be done before the manager is created.
func (m *MonitoringStackFeatureFlag) PreManagerSetupHandle(ctx context.Context) error {
	_ = obov1alpha1.AddToScheme(m.SchemeToUpdate)
	return nil
}

// PostManagerSetupHandle uses the manager's cached client and scheme to set up the monitoringStackReconcilerOpt
// addonReconcilerOpts w.r.t this featureFlagHandler
func (m *MonitoringStackFeatureFlag) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error {
	*m.AddonReconcilerOptsToUpdate = append(*m.AddonReconcilerOptsToUpdate, addoncontroller.WithMonitoringStackReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	})
	return nil
}

// Enable is ONLY used for Testing. It adds the GetFeatureFlagIdentifier to the
// FeatureFlags in the AddonOperator Spec. If the FeatureFlags field is changed,
// the AddonOperator reconciler exits which triggers the addon operator manager
// to restart with the new configuration.
func (m *MonitoringStackFeatureFlag) Enable(ctx context.Context) error {
	return EnableFeatureFlag(ctx, m.Client, m.GetFeatureFlagIdentifier())
}

// Disable is ONLY used for Testing. It removes the GetFeatureFlagIdentifier from the
// FeatureFlags in the AddonOperator Spec if it exists. If the FeatureFlags field is changed,
// // the AddonOperator reconciler exits which triggers the addon operator manager
// // to restart with the new configuration.
func (m *MonitoringStackFeatureFlag) Disable(ctx context.Context) error {
	return DisableFeatureFlag(ctx, m.Client, m.GetFeatureFlagIdentifier())
}

// PreClusterCreationSetup is ONLY used for testing. It preforms any set up needed before
// the test cluster is created.
func (m *MonitoringStackFeatureFlag) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

// PostClusterCreationSetup is ONLY used for test. It preforms any set up needed after
// the test cluster is created.
func (m *MonitoringStackFeatureFlag) PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error {
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

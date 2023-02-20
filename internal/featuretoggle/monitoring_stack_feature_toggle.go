package featuretoggle

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/mt-sre/devkube/dev"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

var _ FeatureToggleHandler = (*MonitoringStackFeatureToggle)(nil)

var observabilityOperatorVersion = "0.0.15"

type MonitoringStackFeatureToggle struct {
	Client                      client.Client
	FeatureTogglesInCluster     addonsv1alpha1.AddonOperatorFeatureToggles
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (m MonitoringStackFeatureToggle) Name() string {
	return "Monitoring Stack Reconciliation Feature Toggle"
}

func (m MonitoringStackFeatureToggle) IsEnabled() bool {
	return m.FeatureTogglesInCluster.ExperimentalFeatures
}

func (m MonitoringStackFeatureToggle) IsEnabledOnTestEnv() bool {
	enabled, ok := os.LookupEnv("EXPERIMENTAL_FEATURES")
	return ok && enabled == "true"
}

func (m *MonitoringStackFeatureToggle) Enable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureToggles: addonsv1alpha1.AddonOperatorFeatureToggles{
						ExperimentalFeatures: true,
					},
				},
			}
			if err := m.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			m.FeatureTogglesInCluster = adoObject.Spec.FeatureToggles
			return nil
		}
		return err
	}
	// no need to do anything if its already enabled
	if adoInCluster.Spec.FeatureToggles.ExperimentalFeatures {
		return nil
	}
	adoInCluster.Spec.FeatureToggles.ExperimentalFeatures = true
	if err := m.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	m.FeatureTogglesInCluster = adoInCluster.Spec.FeatureToggles
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

func (m *MonitoringStackFeatureToggle) Disable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureToggles: addonsv1alpha1.AddonOperatorFeatureToggles{
						ExperimentalFeatures: false,
					},
				},
			}
			if err := m.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			m.FeatureTogglesInCluster = adoObject.Spec.FeatureToggles
			return nil
		}
		return err
	}
	// no need to do anything if its already disabled
	if !adoInCluster.Spec.FeatureToggles.ExperimentalFeatures {
		return nil
	}
	adoInCluster.Spec.FeatureToggles.ExperimentalFeatures = false
	if err := m.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	m.FeatureTogglesInCluster = adoInCluster.Spec.FeatureToggles
	return nil
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

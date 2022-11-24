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

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

var _ FeatureToggleHandler = (*MonitoringStackFeatureToggle)(nil)

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

func (m *MonitoringStackFeatureToggle) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

func (m *MonitoringStackFeatureToggle) PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error {
	// Install Monitoring CRDs for Observability Operator.
	// and deploy the observability operator

	return nil
}

func (m *MonitoringStackFeatureToggle) PreManagerSetupHandle(ctx context.Context) error {
	// nothing to handle before the manager is setup
	// add the MonitoringStack scheme to `m.SchemeToUpdate`
	return nil
}

func (m *MonitoringStackFeatureToggle) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error {
	// use the manager's cached client and scheme to setup the monitoringStackReconcilerOpt addonReconcilerOpts w.r.t this featureToggleHandler
	// *m.AddonReconcilerOptsToUpdate = append(*m.AddonReconcilerOptsToUpdate, addoncontroller.WithMonitoringStackReconciler{
	// 	Client: mgr.GetClient(),
	// 	Scheme: mgr.GetScheme(),
	// })
	return nil
}

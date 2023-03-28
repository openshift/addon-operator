package featuretoggle

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"sigs.k8s.io/controller-runtime/pkg/client"

	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

var _ FeatureToggleHandler = (*MonitoringStackFeatureToggle)(nil)

type MonitoringStackFeatureToggle struct {
	Client                      client.Client
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (m MonitoringStackFeatureToggle) Name() string {
	return "Monitoring Stack Reconciliation Feature Toggle"
}

func (m MonitoringStackFeatureToggle) GetFeatureToggleIdentifier() string {
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

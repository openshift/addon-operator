package featureflag

import (
	"context"

	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

const MonitoringStackFeatureFlagIdentifier = "EXPERIMENTAL_FEATURES"

var _ Handler = (*MonitoringStackFeatureFlag)(nil)

type MonitoringStackFeatureFlag struct {
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (m *MonitoringStackFeatureFlag) Name() string {
	return "Monitoring Stack Reconciliation Feature Flag"
}

func (m *MonitoringStackFeatureFlag) GetFeatureFlagIdentifier() string {
	return MonitoringStackFeatureFlagIdentifier
}

// PreManagerSetupHandle sets up to be done before the manager is created.
func (m *MonitoringStackFeatureFlag) PreManagerSetupHandle(ctx context.Context, scheme *runtime.Scheme) {
	_ = obov1alpha1.AddToScheme(m.SchemeToUpdate)
}

// PostManagerSetupHandle uses the manager's cached client and scheme to set up the monitoringStackReconcilerOpt
// addonReconcilerOpts w.r.t this featureFlagHandler
func (m *MonitoringStackFeatureFlag) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) *[]addoncontroller.AddonReconcilerOptions {
	reconcilerOptions := []addoncontroller.AddonReconcilerOptions{
		addoncontroller.WithMonitoringStackReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
	}
	return &reconcilerOptions
}

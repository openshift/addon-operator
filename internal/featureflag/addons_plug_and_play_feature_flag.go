package featureflag

import (
	"context"
	"fmt"
	"time"

	"github.com/mt-sre/devkube/dev"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

const pkoVersion = "1.6.1"
const AddonsPlugAndPlayFeatureFlagIdentifier = "ADDONS_PLUG_AND_PLAY"

var _ Handler = (*AddonsPlugAndPlayFeatureFlag)(nil)

type AddonsPlugAndPlayFeatureFlag struct {
	Handler
	Client                      client.Client
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (h *AddonsPlugAndPlayFeatureFlag) Name() string {
	return "Addons Plug And Play Feature Flag"
}

func (h *AddonsPlugAndPlayFeatureFlag) GetFeatureFlagIdentifier() string {
	return AddonsPlugAndPlayFeatureFlagIdentifier
}

func (h *AddonsPlugAndPlayFeatureFlag) PreManagerSetupHandle(ctx context.Context) error {
	_ = pkov1alpha1.AddToScheme(h.SchemeToUpdate)
	return nil
}

func (h *AddonsPlugAndPlayFeatureFlag) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error {
	*h.AddonReconcilerOptsToUpdate = append(*h.AddonReconcilerOptsToUpdate, addoncontroller.WithPackageOperatorReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	})
	return nil
}

// Enable is ONLY used for Testing. It adds the GetFeatureFlagIdentifier to the
// FeatureFlags in the AddonOperator Spec. If the `FeatureFlags` field is changed,
// the AddonOperator reconciler exits which triggers the addon operator manager
// to restart with the new configuration.
func (h *AddonsPlugAndPlayFeatureFlag) Enable(ctx context.Context) error {
	return EnableFeatureFlag(ctx, h.Client, h.GetFeatureFlagIdentifier())
}

// Disable is ONLY used for Testing. It removes the GetFeatureFlagIdentifier from the
// FeatureFlags in the AddonOperator Spec if it exists. If the FeatureFlags field is changed,
// // the AddonOperator reconciler exits which triggers the addon operator manager
// // to restart with the new configuration.
func (h *AddonsPlugAndPlayFeatureFlag) Disable(ctx context.Context) error {
	return DisableFeatureFlag(ctx, h.Client, h.GetFeatureFlagIdentifier())
}

// PreClusterCreationSetup is ONLY used for testing. It preforms any set up needed before
// the test cluster is created.
func (h *AddonsPlugAndPlayFeatureFlag) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

// PostClusterCreationSetup is ONLY used for test. It preforms any set up needed after
// the test cluster is created.
func (h *AddonsPlugAndPlayFeatureFlag) PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error {
	if err := clusterCreated.CreateAndWaitFromHttp(ctx, []string{
		"https://github.com/package-operator/package-operator/releases/download/v" + pkoVersion + "/self-bootstrap-job.yaml",
	}); err != nil {
		return fmt.Errorf("install PKO: %w", err)
	}

	deployment := &appsv1.Deployment{}
	deployment.SetNamespace("package-operator-system")
	deployment.SetName("package-operator-manager")

	if err := clusterCreated.Waiter.WaitForCondition(
		ctx, deployment, "Available", metav1.ConditionTrue,
		dev.WithInterval(10*time.Second), dev.WithTimeout(5*time.Minute),
	); err != nil {
		return fmt.Errorf("waiting for PKO installation: %w", err)
	}
	return nil
}

package featuretoggle

import (
	"context"
	"fmt"
	"time"

	"github.com/mt-sre/devkube/dev"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

const pkoVersion = "1.4.0"
const AddonsPlugAndPlayFeatureToggleIdentifier = "ADDONS_PLUG_AND_PLAY"

var _ Handler = (*AddonsPlugAndPlayFeatureToggle)(nil)

type AddonsPlugAndPlayFeatureToggle struct {
	Handler
	Client                      client.Client
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (h *AddonsPlugAndPlayFeatureToggle) Name() string {
	return "Addons Plug And Play Feature Toggle"
}

func (h *AddonsPlugAndPlayFeatureToggle) GetFeatureToggleIdentifier() string {
	return AddonsPlugAndPlayFeatureToggleIdentifier
}

func (h *AddonsPlugAndPlayFeatureToggle) PreManagerSetupHandle(ctx context.Context) error {
	_ = pkov1alpha1.AddToScheme(h.SchemeToUpdate)
	return nil
}

func (h *AddonsPlugAndPlayFeatureToggle) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error {
	*h.AddonReconcilerOptsToUpdate = append(*h.AddonReconcilerOptsToUpdate, addoncontroller.WithPackageOperatorReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	})
	return nil
}

// Enable is ONLY used for Testing. It adds the GetFeatureToggleIdentifier to the
// FeatureToggles in the AddonOperator Spec. If the FeatureToggles field is changed,
// the AddonOperator reconciler exits which triggers the addon operator manager
// to restart with the new configuration.
func (h *AddonsPlugAndPlayFeatureToggle) Enable(ctx context.Context) error {
	return EnableFeatureToggle(ctx, h.Client, h.GetFeatureToggleIdentifier())
}

// Disable is ONLY used for Testing. It removes the GetFeatureToggleIdentifier from the
// FeatureToggles in the AddonOperator Spec if it exists. If the FeatureToggles field is changed,
// // the AddonOperator reconciler exits which triggers the addon operator manager
// // to restart with the new configuration.
func (h *AddonsPlugAndPlayFeatureToggle) Disable(ctx context.Context) error {
	return DisableFeatureToggle(ctx, h.Client, h.GetFeatureToggleIdentifier())
}

// PreClusterCreationSetup is ONLY used for testing. It preforms any set up needed before
// the test cluster is created.
func (h *AddonsPlugAndPlayFeatureToggle) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

// PostClusterCreationSetup is ONLY used for test. It preforms any set up needed after
// the test cluster is created.
func (h *AddonsPlugAndPlayFeatureToggle) PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error {
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

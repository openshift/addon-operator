package featuretoggle

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

var _ FeatureToggleHandler = (*AddonsPlugAndPlayFeatureToggle)(nil)

type AddonsPlugAndPlayFeatureToggle struct {
	FeatureToggleHandler
	Client                      client.Client
	SchemeToUpdate              *runtime.Scheme
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (h *AddonsPlugAndPlayFeatureToggle) Name() string {
	return "Addons Plug And Play Feature Toggle"
}

func (h *AddonsPlugAndPlayFeatureToggle) GetFeatureToggleIdentifier() string {
	return "ADDONS_PLUG_AND_PLAY"
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

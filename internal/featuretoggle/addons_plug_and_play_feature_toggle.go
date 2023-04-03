package featuretoggle

import (
	"context"

	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

var _ FeatureToggleHandler = (*AddonsPlugAndPlayFeatureToggleHandler)(nil)

type AddonsPlugAndPlayFeatureToggleHandler struct {
	BaseFeatureToggleHandler
}

func (handler AddonsPlugAndPlayFeatureToggleHandler) Name() string {
	return "Addons Plug And Play Feature Toggle"
}

func (handler AddonsPlugAndPlayFeatureToggleHandler) GetFeatureToggleIdentifier() string {
	return "ADDONS_PLUG_AND_PLAY"
}

func (handler *AddonsPlugAndPlayFeatureToggleHandler) PreManagerSetupHandle(ctx context.Context) error {
	_ = pkov1alpha1.AddToScheme(handler.SchemeToUpdate)
	return nil
}

func (handler *AddonsPlugAndPlayFeatureToggleHandler) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) error {
	*handler.AddonReconcilerOptsToUpdate = append(*handler.AddonReconcilerOptsToUpdate, addoncontroller.WithPackageOperatorReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	})
	return nil
}

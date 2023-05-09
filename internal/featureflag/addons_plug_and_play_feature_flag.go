package featureflag

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

const AddonsPlugAndPlayFeatureFlagIdentifier = "ADDONS_PLUG_AND_PLAY"

var _ Handler = (*AddonsPlugAndPlayFeatureFlag)(nil)

type AddonsPlugAndPlayFeatureFlag struct {
}

func (h *AddonsPlugAndPlayFeatureFlag) Name() string {
	return "Addons Plug And Play Feature Flag"
}

func (h *AddonsPlugAndPlayFeatureFlag) GetFeatureFlagIdentifier() string {
	return AddonsPlugAndPlayFeatureFlagIdentifier
}

func (h *AddonsPlugAndPlayFeatureFlag) PreManagerSetupHandle(ctx context.Context, scheme *runtime.Scheme) {
	_ = pkov1alpha1.AddToScheme(scheme)
}

func (h *AddonsPlugAndPlayFeatureFlag) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) *[]addoncontroller.AddonReconcilerOptions {
	reconcilerOptions := []addoncontroller.AddonReconcilerOptions{
		addoncontroller.WithPackageOperatorReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
	}
	return &reconcilerOptions
}

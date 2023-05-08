package featureflag

import (
	"context"
	"strings"

	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

type Getter struct {
	SchemeToUpdate *runtime.Scheme
}

func (g Getter) Get() []Handler {
	return []Handler{
		&MonitoringStackFeatureFlag{},
		&AddonsPlugAndPlayFeatureFlag{},
	}
}

type Handler interface {
	PreManagerSetupHandle(ctx context.Context, scheme *runtime.Scheme)
	PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) *[]addoncontroller.AddonReconcilerOptions
}

type HandlerList []Handler

var _ Handler = (HandlerList)(nil)

func GetFeatureFlagHandler(ado *addonsv1alpha1.AddonOperator) HandlerList {
	handler := HandlerList{}
	if IsEnabled(AddonsPlugAndPlayFeatureFlagIdentifier, *ado) {
		handler = append(handler, &AddonsPlugAndPlayFeatureFlag{})
	}
	if IsEnabled(MonitoringStackFeatureFlagIdentifier, *ado) {
		handler = append(handler, &MonitoringStackFeatureFlag{})
	}
	return handler
}

func (hl HandlerList) PreManagerSetupHandle(ctx context.Context, scheme *runtime.Scheme) {
	for _, h := range hl {
		h.PreManagerSetupHandle(ctx, scheme)
	}
}

func (hl HandlerList) PostManagerSetupHandle(ctx context.Context, mgr manager.Manager) *[]addoncontroller.AddonReconcilerOptions {
	reconcilerOptions := []addoncontroller.AddonReconcilerOptions{}
	for _, h := range hl {
		reconcilerOptions = append(reconcilerOptions, *h.PostManagerSetupHandle(ctx, mgr)...)
	}
	return &reconcilerOptions
}

func IsEnabled(featureFlagIdentifier string, addonOperator addonsv1alpha1.AddonOperator) bool {
	enabledFeatureFlags := addonOperator.Spec.FeatureFlags
	return slices.Contains(strings.Split(enabledFeatureFlags, ","), featureFlagIdentifier)
}

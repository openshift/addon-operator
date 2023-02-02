package featuretoggle

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
)

func GetAvailableFeatureToggles(opts ...availableFeatureTogglesGetterOpts) []FeatureToggleHandler {
	params := &availableFeatureToggleGetterParams{}
	for _, opt := range opts {
		opt.apply(params)
	}

	return []FeatureToggleHandler{
		&MonitoringStackFeatureToggle{
			Client:                      params.client,
			FeatureTogglesInCluster:     params.featureTogglesInCluster,
			SchemeToUpdate:              params.schemeToUpdate,
			AddonReconcilerOptsToUpdate: params.addonReconcilerOptsToUpdate,
		},
	}
}

type availableFeatureTogglesGetterOpts interface {
	apply(*availableFeatureToggleGetterParams)
}

type availableFeatureToggleGetterParams struct {
	client                      client.Client
	featureTogglesInCluster     addonsv1alpha1.AddonOperatorFeatureToggles
	schemeToUpdate              *runtime.Scheme
	addonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

type WithClient struct {
	client.Client
}

func (w WithClient) apply(a *availableFeatureToggleGetterParams) {
	a.client = w.Client
}

type WithSchemeToUpdate struct {
	*runtime.Scheme
}

func (w WithSchemeToUpdate) apply(a *availableFeatureToggleGetterParams) {
	a.schemeToUpdate = w.Scheme
}

type WithFeatureTogglesInCluster addonsv1alpha1.AddonOperatorFeatureToggles

func (w WithFeatureTogglesInCluster) apply(a *availableFeatureToggleGetterParams) {
	a.featureTogglesInCluster = addonsv1alpha1.AddonOperatorFeatureToggles(w)
}

type WithAddonReconcilerOptsToUpdate struct {
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (w WithAddonReconcilerOptsToUpdate) apply(a *availableFeatureToggleGetterParams) {
	a.addonReconcilerOptsToUpdate = w.AddonReconcilerOptsToUpdate
}

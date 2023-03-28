package featuretoggle

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

type WithAddonReconcilerOptsToUpdate struct {
	AddonReconcilerOptsToUpdate *[]addoncontroller.AddonReconcilerOptions
}

func (w WithAddonReconcilerOptsToUpdate) apply(a *availableFeatureToggleGetterParams) {
	a.addonReconcilerOptsToUpdate = w.AddonReconcilerOptsToUpdate
}

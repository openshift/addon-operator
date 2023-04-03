package featuretoggle

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func (handler *BaseFeatureToggleHandler) Enable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := handler.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureToggles: handler.GetFeatureToggleIdentifier(),
				},
			}
			if err := handler.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already enabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureToggles, ",")
	isAddonsPlugAndPlayAlreadyEnabled := stringPresentInSlice(handler.GetFeatureToggleIdentifier(), existingFeatureToggles)
	if isAddonsPlugAndPlayAlreadyEnabled {
		return nil
	}
	adoInCluster.Spec.FeatureToggles += "," + handler.GetFeatureToggleIdentifier()
	if err := handler.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func (handler *BaseFeatureToggleHandler) Disable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := handler.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureToggles: "",
				},
			}
			if err := handler.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already disabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureToggles, ",")
	isAddonsPlugAndPlayAlreadyEnabled := stringPresentInSlice(handler.GetFeatureToggleIdentifier(), existingFeatureToggles)
	if !isAddonsPlugAndPlayAlreadyEnabled {
		return nil
	}
	updatedFeatureToggles := ""
	for _, featTog := range existingFeatureToggles {
		if featTog == handler.GetFeatureToggleIdentifier() {
			continue
		}
		updatedFeatureToggles += "," + featTog
	}
	if len(updatedFeatureToggles) != 0 {
		updatedFeatureToggles = updatedFeatureToggles[1:]
	}
	adoInCluster.Spec.FeatureToggles = updatedFeatureToggles
	if err := handler.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

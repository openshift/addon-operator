package featuretoggle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/mt-sre/devkube/dev"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var pkoVersion = "1.4.0"

func (h *AddonsPlugAndPlayFeatureToggle) Enable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := h.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureToggles: h.GetFeatureToggleIdentifier(),
				},
			}
			if err := h.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already enabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureToggles, ",")
	isAddonsPlugAndPlayAlreadyEnabled := stringPresentInSlice(h.GetFeatureToggleIdentifier(), existingFeatureToggles)
	if isAddonsPlugAndPlayAlreadyEnabled {
		return nil
	}
	adoInCluster.Spec.FeatureToggles += "," + h.GetFeatureToggleIdentifier()
	if err := h.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func (h *AddonsPlugAndPlayFeatureToggle) Disable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := h.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureToggles: "",
				},
			}
			if err := h.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already disabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureToggles, ",")
	isAddonsPlugAndPlayAlreadyEnabled := stringPresentInSlice(h.GetFeatureToggleIdentifier(), existingFeatureToggles)
	if !isAddonsPlugAndPlayAlreadyEnabled {
		return nil
	}
	updatedFeatureToggles := ""
	for _, featTog := range existingFeatureToggles {
		if featTog == h.GetFeatureToggleIdentifier() {
			continue
		}
		updatedFeatureToggles += "," + featTog
	}
	if len(updatedFeatureToggles) != 0 {
		updatedFeatureToggles = updatedFeatureToggles[1:]
	}
	adoInCluster.Spec.FeatureToggles = updatedFeatureToggles
	if err := h.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func (h *AddonsPlugAndPlayFeatureToggle) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

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

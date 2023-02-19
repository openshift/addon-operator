package featuretoggle

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/mt-sre/devkube/dev"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

func (m *MonitoringStackFeatureToggle) Enable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureToggles: m.GetFeatureToggleIdentifier(),
				},
			}
			if err := m.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already enabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureToggles, ",")
	isMonitoringStackAlreadyEnabled := stringPresentInSlice(m.GetFeatureToggleIdentifier(), existingFeatureToggles)
	if isMonitoringStackAlreadyEnabled {
		return nil
	}
	adoInCluster.Spec.FeatureToggles += "," + m.GetFeatureToggleIdentifier()
	if err := m.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func (m *MonitoringStackFeatureToggle) Disable(ctx context.Context) error {
	adoInCluster := addonsv1alpha1.AddonOperator{}
	if err := m.Client.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &adoInCluster); err != nil {
		if errors.IsNotFound(err) {
			adoObject := addonsv1alpha1.AddonOperator{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonsv1alpha1.DefaultAddonOperatorName,
				},
				Spec: addonsv1alpha1.AddonOperatorSpec{
					FeatureToggles: "",
				},
			}
			if err := m.Client.Create(ctx, &adoObject); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	// no need to do anything if its already disabled
	existingFeatureToggles := strings.Split(adoInCluster.Spec.FeatureToggles, ",")
	isMonitoringStackAlreadyEnabled := stringPresentInSlice(m.GetFeatureToggleIdentifier(), existingFeatureToggles)
	if !isMonitoringStackAlreadyEnabled {
		return nil
	}
	updatedFeatureToggles := ""
	for _, featTog := range existingFeatureToggles {
		if featTog == m.GetFeatureToggleIdentifier() {
			continue
		}
		updatedFeatureToggles += "," + featTog
	}
	if len(updatedFeatureToggles) != 0 {
		updatedFeatureToggles = updatedFeatureToggles[1:]
	}
	adoInCluster.Spec.FeatureToggles = updatedFeatureToggles
	if err := m.Client.Update(ctx, &adoInCluster); err != nil {
		return fmt.Errorf("failed to enable the feature toggle in the AddonOperator object: %w", err)
	}
	return nil
}

func (m *MonitoringStackFeatureToggle) PreClusterCreationSetup(ctx context.Context) error {
	return nil
}

func (m *MonitoringStackFeatureToggle) PostClusterCreationSetup(ctx context.Context, clusterCreated *dev.Cluster) error {
	// Install Monitoring CRDs for Observability Operator.
	// and deploy the observability operator

	return nil
}

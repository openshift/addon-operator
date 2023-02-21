package integration_test

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"
	"github.com/openshift/addon-operator/internal/featuretoggle"
)

func (s *integrationTestSuite) TestMonitoringStack_MonitoringInPlaceAtCreationWithAvailableState() {
	if !featuretoggle.IsEnabledOnTestEnv(&featuretoggle.MonitoringStackFeatureToggle{}) {
		s.T().Skip("skipping Monitoring Stack Integration tests as the feature toggle for it is disabled in the test environment")
	}

	ctx := context.Background()

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "addon-41b95034425c4d55",
		},
		Spec: addonsv1alpha1.AddonSpec{
			DisplayName: "addon-41b95034425c4d55",
			Namespaces: []addonsv1alpha1.AddonNamespace{
				{Name: "namespace-a9953682ff70d594"},
			},
			Install: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						Namespace:          "namespace-a9953682ff70d594",
						CatalogSourceImage: referenceAddonCatalogSourceImageWorking,
						Channel:            "alpha",
						PackageName:        "reference-addon",
					},
				},
			},
			Monitoring: &addonsv1alpha1.MonitoringSpec{
				MonitoringStack: &addonsv1alpha1.MonitoringStackSpec{
					RHOBSRemoteWriteConfig: &addonsv1alpha1.RHOBSRemoteWriteConfigSpec{
						URL: "prometheus-remote-storage-mock.prometheus-remote-storage-mock:1234",
					},
				},
			},
		},
	}

	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)

	// clean up addon resource in case it
	// was leaked because of a failed test
	s.T().Cleanup(func() {
		s.addonCleanup(addon, ctx)
	})

	// wait until Addon is available
	err = integration.WaitForObject(
		ctx,
		s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
		func(obj client.Object) (done bool, err error) {
			a := obj.(*addonsv1alpha1.Addon)
			return meta.IsStatusConditionTrue(
				a.Status.Conditions, addonsv1alpha1.Available), nil
		})
	s.Require().NoError(err)

	reconciledMonitoringStack := &obov1alpha1.MonitoringStack{}
	err = integration.Client.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-monitoring-stack", addon.Name), Namespace: "namespace-a9953682ff70d594"}, reconciledMonitoringStack)
	s.Require().NoError(err)

	availableCondition, reconciledCondition := obov1alpha1.Condition{}, obov1alpha1.Condition{}
	availableConditionFound, reconciledConditionFound := false, false

	for _, cond := range reconciledMonitoringStack.Status.Conditions {
		cond := cond
		if cond.Type == obov1alpha1.AvailableCondition {
			availableCondition = cond
			availableConditionFound = true
		} else if cond.Type == obov1alpha1.ReconciledCondition {
			reconciledCondition = cond
			reconciledConditionFound = true
		}
		if availableConditionFound && reconciledConditionFound {
			break
		}
	}

	s.Require().True(availableConditionFound)
	s.Require().True(reconciledConditionFound)

	s.Require().Equal(obov1alpha1.ConditionTrue, availableCondition.Status)
	s.Require().Equal(obov1alpha1.ConditionTrue, reconciledCondition.Status)
}

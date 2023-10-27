package integration_test

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/integration"
)

func (s *integrationTestSuite) TestAddon_AddonInstance() {
	addonOwnNamespace := addon_OwnNamespace()
	addonAllNamespaces := addon_AllNamespaces()

	tests := []struct {
		name            string
		targetNamespace string
		addon           *addonsv1alpha1.Addon
	}{
		{
			name:            "OwnNamespace",
			addon:           addonOwnNamespace,
			targetNamespace: addonOwnNamespace.Spec.Install.OLMOwnNamespace.Namespace,
		},
		{
			name:            "AllNamespaces",
			addon:           addonAllNamespaces,
			targetNamespace: addonAllNamespaces.Spec.Install.OLMAllNamespaces.Namespace,
		},
	}
	for _, test := range tests {
		s.Run(test.name, func() {
			ctx := context.Background()
			addon := test.addon

			err := integration.Client.Create(ctx, addon)
			s.Require().NoError(err)
			s.T().Cleanup(func() {
				s.addonCleanup(test.addon, ctx)
			})

			err = integration.WaitForObject(
				ctx,
				s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
				func(obj client.Object) (done bool, err error) {
					a := obj.(*addonsv1alpha1.Addon)
					return meta.IsStatusConditionTrue(
						a.Status.Conditions, addonsv1alpha1.Available), nil
				})
			s.Require().NoError(err)

			// check that there is an addonInstance in the target namespace.
			addonInstance := &addonsv1alpha1.AddonInstance{}
			err = integration.Client.Get(ctx, client.ObjectKey{
				Name:      addonsv1alpha1.DefaultAddonInstanceName,
				Namespace: test.targetNamespace,
			}, addonInstance)
			s.Require().NoError(err)
			// Default of 10s is hardcoded in AddonInstanceReconciler
			s.Assert().Equal(10*time.Second, addonInstance.Spec.HeartbeatUpdatePeriod.Duration)
		})
	}
}

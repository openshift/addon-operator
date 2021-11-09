package integration_test

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	addoninstanceapi "github.com/openshift/addon-operator/clients/addoninstance"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
				func(obj client.Object) (done bool, err error) {
					a := obj.(*addonsv1alpha1.Addon)
					return meta.IsStatusConditionTrue(
						a.Status.Conditions, addonsv1alpha1.Available), nil
				})
			s.NoError(err)

			// check that there is an addonInstance in the target namespace.
			addonInstance := &addonsv1alpha1.AddonInstance{}
			err = integration.Client.Get(ctx, client.ObjectKey{
				Name:      addonsv1alpha1.DefaultAddonInstanceName,
				Namespace: test.targetNamespace,
			}, addonInstance)

			s.NoError(err)

			// initially there shouldn't be any .status.conditions under the addonInstance
			s.Require().Zero(len(addonInstance.Status.Conditions))

			healthyHeartbeatCondition := metav1.Condition{
				Type:    "addons.managed.openshift.io/Healthy",
				Status:  "True",
				Reason:  "ComponentsUp",
				Message: "Reference Addon is operational",
			}

			// send a heartbeat for the above addon to its corresponding addonInstance
			err = addoninstanceapi.SetAddonInstanceCondition(ctx, integration.Client, healthyHeartbeatCondition, addon.Name)
			s.NoError(err)

			err = integration.Client.Get(ctx, client.ObjectKey{
				Name:      addonsv1alpha1.DefaultAddonInstanceName,
				Namespace: test.targetNamespace,
			}, addonInstance)
			s.NoError(err)
			s.Require().True(meta.IsStatusConditionTrue(addonInstance.Status.Conditions, "addons.managed.openshift.io/Healthy"))

			// waiting for heartbeat to expire
			time.Sleep(40 * time.Second)

			// check the addoninstance resource again to see if the heartbeat expired or not. Expected: it should be expired
			err = integration.Client.Get(ctx, client.ObjectKey{
				Name:      addonsv1alpha1.DefaultAddonInstanceName,
				Namespace: test.targetNamespace,
			}, addonInstance)
			s.NoError(err)

			latestCondition := meta.FindStatusCondition(addonInstance.Status.Conditions, "addons.managed.openshift.io/Healthy")
			s.Require().Equal(addoninstanceapi.HeartbeatTimeoutCondition, metav1.Condition{
				Type:    latestCondition.Type,
				Status:  latestCondition.Status,
				Reason:  latestCondition.Reason,
				Message: latestCondition.Message,
			})
		})
	}
}

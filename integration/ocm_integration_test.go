package integration_test

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"
	addonutils "github.com/openshift/addon-operator/internal/controllers/addon"
	"github.com/openshift/addon-operator/internal/ocm"
	"github.com/openshift/addon-operator/internal/testutil"
)

func (s *integrationTestSuite) TestUpgradePolicyReporting() {
	if !testutil.IsApiMockEnabled() {
		s.T().Skip("skipping OCM tests since api mock execution is disabled")
	}

	ctx := context.Background()
	addon := addon_OwnNamespace_UpgradePolicyReporting()

	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		s.addonCleanup(addon, ctx)
	})

	_, err = integration.OCMClient.PatchUpgradePolicy(ctx, ocm.UpgradePolicyPatchRequest{
		ID:    addon.Spec.UpgradePolicy.ID,
		Value: ocm.UpgradePolicyValueScheduled,
	})
	s.Require().NoError(err)

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

	s.Run("reports to upgrade policy endpoint", func() {
		res, err := integration.OCMClient.GetUpgradePolicy(ctx, ocm.UpgradePolicyGetRequest{ID: addon.Spec.UpgradePolicy.ID})
		s.Require().NoError(err)

		s.Assert().Equal(ocm.UpgradePolicyValueCompleted, res.Value)
	})
}

func (s *integrationTestSuite) TestAddonStatusReporting() {
	if !testutil.IsApiMockEnabled() {
		s.T().Skip("skipping OCM tests since api mock execution is disabled")
	}
	ctx := context.Background()

	addon := addon_OwnNamespace()
	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		s.addonCleanup(addon, ctx)
	})

	// wait until Addon is availablez
	err = integration.WaitForObject(
		ctx,
		s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
		func(obj client.Object) (done bool, err error) {
			a := obj.(*addonsv1alpha1.Addon)
			return meta.IsStatusConditionTrue(
				a.Status.Conditions, addonsv1alpha1.Available), nil
		})
	s.Require().NoError(err)

	s.Run("successfully reports to the addon status API", func() {
		res, err := integration.OCMClient.GetAddOnStatus(ctx, addon.Name)
		s.Require().NoError(err)
		s.Require().Equal(addon.Name, res.AddonID)
		s.Require().Equal(len(res.StatusConditions), 2)

		expectedConditions := map[string]string{
			addonsv1alpha1.Available: string(metav1.ConditionTrue),
			addonsv1alpha1.Installed: string(metav1.ConditionTrue),
		}
		for _, condition := range res.StatusConditions {
			expectedVal, found := expectedConditions[condition.StatusType]
			s.Require().True(found && expectedVal == string(condition.StatusValue))
		}
	})

	s.Run("patches the addon status API when new conditions are reported", func() {
		addon.Spec.Paused = true
		err = integration.Client.Update(ctx, addon)
		s.Require().NoError(err)
		// wait until Addon is paused.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to be paused",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.Paused), nil
			})
		s.Require().NoError(err)

		// Assert that the paused condition is reported.
		res, err := integration.OCMClient.GetAddOnStatus(ctx, addon.Name)
		s.Require().NoError(err)
		s.Require().Equal(addon.Name, res.AddonID)
		s.Require().Equal(len(res.StatusConditions), 3)
		expectedConditions := map[string]string{
			addonsv1alpha1.Available: string(metav1.ConditionTrue),
			addonsv1alpha1.Installed: string(metav1.ConditionTrue),
			addonsv1alpha1.Paused:    string(metav1.ConditionTrue),
		}

		for _, condition := range res.StatusConditions {
			expectedVal, found := expectedConditions[condition.StatusType]
			s.Require().True(found && expectedVal == string(condition.StatusValue))
		}

		// Assert that the addon object correctly updates its status block with the
		// last reported status hash.
		err = integration.Client.Get(ctx, types.NamespacedName{
			Name:      addon.Name,
			Namespace: addon.Namespace,
		}, addon)
		s.Require().NoError(err)

		s.Require().NotNil(addon.Status.OCMReportedStatusHash)
		s.Require().Equal(addon.Status.OCMReportedStatusHash.StatusHash, addonutils.HashCurrentAddonStatus(addon))
	})

}

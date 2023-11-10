package integration_test

import (
	"context"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/integration"
)

func (s *integrationTestSuite) Teardown() {
	ctx := context.Background()

	// assert that all addons are gone before teardown
	addonList := &addonsv1alpha1.AddonList{}
	err := integration.Client.List(ctx, addonList)
	s.Assert().NoError(err)
	addonNames := []string{}
	for _, a := range addonList.Items {
		addonNames = append(addonNames, a.GetName())
	}
	s.Assert().Len(addonNames, 0, "expected all Addons to be gone before teardown, but some still exist")
}

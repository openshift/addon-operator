package integration_test

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"
)

func (s *integrationTestSuite) Setup() {
	ctx := context.Background()

	s.Run("AddonOperator available", func() {
		addonOperator := addonsv1alpha1.AddonOperator{}

		// Wait for API to be created
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := integration.Client.Get(ctx, client.ObjectKey{
				Name: addonsv1alpha1.DefaultAddonOperatorName,
			}, &addonOperator)
			return err
		})
		s.Require().NoError(err)

		err = integration.WaitForObject(
			s.T(), defaultAddonAvailabilityTimeout, &addonOperator, "to be Available",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.AddonOperator)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.Available), nil
			})
		s.Require().NoError(err)
	})

	s.Run("Patch AddonOperator with OCM mock configuration", func() {
		addonOperator := &addonsv1alpha1.AddonOperator{}
		if err := integration.Client.Get(ctx, client.ObjectKey{
			Name: addonsv1alpha1.DefaultAddonOperatorName,
		}, addonOperator); err != nil {
			s.T().Fatalf("get AddonOperator object: %v", err)
		}

		addonOperator.Spec.OCM = &addonsv1alpha1.AddonOperatorOCM{
			Endpoint: integration.OCMAPIEndpoint,
			Secret: addonsv1alpha1.ClusterSecretReference{
				Name:      "pull-secret",
				Namespace: "api-mock",
			},
		}
		if err := integration.Client.Update(ctx, addonOperator); err != nil {
			s.T().Fatalf("patch AddonOperator object: %v", err)
		}
	})
}

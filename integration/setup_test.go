package integration_test

import (
	"context"
	"fmt"

	"github.com/openshift/addon-operator/internal/ocm"

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

	// creating the OCMClient after applying all deployments, as ocm.NewClient() will
	// talk to the OCM API to resolve the ClusterID from the ClusterExternalID
	OCMClient, err := ocm.NewClient(
		context.Background(),
		ocm.WithEndpoint("http://127.0.0.1:8001/api/v1/namespaces/api-mock/services/api-mock:80/proxy"),
		ocm.WithAccessToken("accessToken"), //TODO: Needs to be supplied from the outside, does not matter for mock.
		ocm.WithClusterExternalID(string(integration.Cv.Spec.ClusterID)),
	)
	if err != nil {
		panic(fmt.Errorf("initializing ocm client: %w", err))
	}

	integration.OCMClient = OCMClient

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
				Name:      "api-mock",
				Namespace: "api-mock",
			},
		}
		if err := integration.Client.Update(ctx, addonOperator); err != nil {
			s.T().Fatalf("patch AddonOperator object: %v", err)
		}
	})
}

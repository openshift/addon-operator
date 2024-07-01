package integration_test

import (
	"context"
	"log"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/util/retry"

	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/integration"
)

type integrationTestSuite struct {
	suite.Suite
}

func (s *integrationTestSuite) SetupSuite() {
	if !testing.Short() {
		ctx := context.Background()
		addonOperator := addonsv1alpha1.AddonOperator{}

		// Wait for API to be created
		s.T().Log("Checking AddonOperator is created")
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := integration.Client.Get(ctx, client.ObjectKey{
				Name: addonsv1alpha1.DefaultAddonOperatorName,
			}, &addonOperator)
			return err
		})
		s.Require().NoError(err)
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, &addonOperator, "to be Available",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.AddonOperator)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.Available), nil
			})
		s.Require().NoError(err)
		s.T().Log("AddonOperator exists")

		s.T().Log("Patch AddonOperator with OCM mock configuration")
		addonOperatorPointer := &addonsv1alpha1.AddonOperator{}
		if err := integration.Client.Get(ctx, client.ObjectKey{
			Name: addonsv1alpha1.DefaultAddonOperatorName,
		}, addonOperatorPointer); err != nil {
			s.T().Fatalf("get AddonOperator object: %v", err)
		}

		addonOperatorPointer.Spec.OCM = &addonsv1alpha1.AddonOperatorOCM{
			Endpoint: integration.OCMAPIEndpoint,
			Secret: addonsv1alpha1.ClusterSecretReference{
				Name:      "pull-secret",
				Namespace: "api-mock",
			},
		}
		if err := integration.Client.Update(ctx, addonOperatorPointer); err != nil {
			s.T().Fatalf("patch AddonOperator object: %v", err)
		}
		s.T().Log("AddonOperator patched")
	}
}

func (s *integrationTestSuite) TearDownSuite() {
	if !testing.Short() {
		ctx := context.Background()

		// assert that all addons are gone before teardown
		addonList := &addonsv1alpha1.AddonList{}
		err := integration.Client.List(ctx, addonList)
		s.Assert().NoError(err)
		addonNames := []string{}
		for _, a := range addonList.Items {
			addonNames = append(addonNames, a.GetName())
			addon := a
			s.T().Logf("Cleaning up addons from the tear down: %s", addonNames)
			s.addonCleanup(&addon, ctx)
		}
		s.Assert().Len(addonNames, 0, "expected all Addons to be gone before teardown, but some still exist. Cleaned up all of them")
	}

	if err := integration.PrintPodStatusAndLogs("addon-operator"); err != nil {
		log.Fatal(err)
	}
}

func (s *integrationTestSuite) addonCleanup(addon *addonsv1alpha1.Addon,
	ctx context.Context) {
	s.T().Logf("waiting for addon %s to be deleted", addon.Name)

	// delete Addon
	err := integration.Client.Delete(ctx, addon, client.PropagationPolicy("Foreground"))
	s.Require().NoError(client.IgnoreNotFound(err), "delete Addon: %v", addon)

	// wait until Addon is gone
	err = integration.WaitToBeGone(ctx, s.T(), defaultAddonDeletionTimeout, addon)
	s.Require().NoError(err, "wait for Addon to be deleted")
	s.T().Logf("Addon %s is deleted", addon.Name)
}

func TestIntegration(t *testing.T) {
	// Run kube-apiserver proxy during tests
	t.Log("Integration testing starts, setting the proxy")
	apiProxyCloseCh := make(chan struct{})
	defer close(apiProxyCloseCh)
	if err := integration.RunAPIServerProxy(apiProxyCloseCh); err != nil {
		t.Fatal(err)
	}

	t.Log("Integration testing starts, setting the OCMclient")
	// Initialize the OCM client after the proxy has started.
	if err := integration.InitOCMClient(); err != nil {
		t.Fatal(err)
	}

	// does not support parallel test runs
	suite.Run(t, new(integrationTestSuite))
}

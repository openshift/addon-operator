package integration_test

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/openshift/addon-operator/internal/featuretoggle"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"
)

const (
	addonName              = "addonname-pko-boatboat"
	addonNamespace         = "namespace-onbgdions"
	pkoImageOptionalParams = "quay.io/alcosta/package-operator-packages/openshift/addon-operator/apnp-test-optional-params:v2.0"
	pkoImageRequiredParams = "quay.io/alcosta/package-operator-packages/openshift/addon-operator/apnp-test-required-params:v2.0"
	pkoDeploymentNamespace = "default"
	deadMansSnitchUrlValue = "https://example.com/test-snitch-url"
	pagerDutyKeyValue      = "1234567890ABCDEF"
)

func (s *integrationTestSuite) TestPackageOperatorAddon() {
	if !featuretoggle.IsEnabledOnTestEnv(&featuretoggle.AddonsPlugAndPlayFeatureToggle{}) {
		s.T().Skip("skipping PackageOperatorReconciler integration tests as the feature toggle for it is disabled in the test environment")
	}

	tests := []struct {
		name                       string
		pkoImage                   string
		deployDeadMansSnitchSecret bool
		deployPagerDutySecret      bool
	}{
		{"OptionalParamsAllMissing", pkoImageOptionalParams, false, false},
		{"OptionalParamsAnyMissing", pkoImageOptionalParams, true, false},
		{"OptionalParamsAllPresent", pkoImageOptionalParams, true, true},
		{"RequiredParamsAllMissing", pkoImageRequiredParams, false, false},
		{"RequiredParamsAnyMissing", pkoImageRequiredParams, true, false},
		{"RequiredParamsAllPresent", pkoImageRequiredParams, true, true},
	}

	for index, test := range tests {
		s.Run(test.name, func() {
			testAddonName := fmt.Sprintf("%s-%d", addonName, index)
			testAddonNamespace := fmt.Sprintf("%s-%d", addonNamespace, index)
			ctx := context.Background()

			s.createAddon(ctx, testAddonName, testAddonNamespace, test.pkoImage)
			s.waitForNamespace(ctx, testAddonNamespace)

			if test.deployDeadMansSnitchSecret {
				s.createDeadMansSnitchSecret(ctx, testAddonName, testAddonNamespace)
			}
			if test.deployPagerDutySecret {
				s.createPagerDutySecret(ctx, testAddonName, testAddonNamespace)
			}

			// s.waitForValidDeployment(ctx, testAddonName, test.deployDeadMansSnitchSecret, test.deployPagerDutySecret)

			// s.T().Cleanup(func() { s.addonCleanup(addon, ctx) })
		})
	}
}

// create the Addon resource
func (s *integrationTestSuite) createAddon(ctx context.Context, addonName string, addonNamespace string, pkoImage string) *addonsv1alpha1.Addon {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: addonName},
		Spec: addonsv1alpha1.AddonSpec{
			Version:              "1.0",
			DisplayName:          addonName,
			AddonPackageOperator: &addonsv1alpha1.AddonPackageOperator{Image: pkoImage},
			Namespaces:           []addonsv1alpha1.AddonNamespace{{Name: addonNamespace}},
			Install: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						Namespace:          addonNamespace,
						CatalogSourceImage: referenceAddonCatalogSourceImageWorking,
						Channel:            "alpha",
						PackageName:        "reference-addon",
						Config:             &addonsv1alpha1.SubscriptionConfig{EnvironmentVariables: referenceAddonConfigEnvObjects},
					},
				},
			},
		},
	}

	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)

	return addon
}

// wait for the Addon addonNamespace to exist (needed to publish secrets)
func (s *integrationTestSuite) waitForNamespace(ctx context.Context, addonNamespace string) {
	err := integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: addonNamespace}}, "to be created",
		func(obj client.Object) (done bool, err error) { return true, nil })
	s.Require().NoError(err)
}

// create the Secret resource for Dead Man's Snitch as defined here:
// - https://mt-sre.github.io/docs/creating-addons/monitoring/deadmanssnitch_integration/#generated-secret
func (s *integrationTestSuite) createDeadMansSnitchSecret(ctx context.Context, addonName string, addonNamespace string) {
	s.createSecret(ctx, addonName+"-deadmanssnitch", addonNamespace, map[string][]byte{"SNITCH_URL": []byte(deadMansSnitchUrlValue)})
}

// create the Secret resource for PagerDuty as defined here:
// - https://mt-sre.github.io/docs/creating-addons/monitoring/pagerduty_integration/
func (s *integrationTestSuite) createPagerDutySecret(ctx context.Context, addonName string, addonNamespace string) {
	s.createSecret(ctx, addonName+"-pagerduty", addonNamespace, map[string][]byte{"PAGERDUTY_KEY": []byte(pagerDutyKeyValue)})
}

func (s *integrationTestSuite) createSecret(ctx context.Context, name string, namespace string, data map[string][]byte) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       data,
	}
	err := integration.Client.Create(ctx, secret)
	s.Require().NoError(err)
}

// wait until all the replicas in the Deployment inside the ClusterPackage are ready
// and check if their env variables corresponds to the secrets
func (s *integrationTestSuite) waitForValidDeployment(ctx context.Context, addonName string,
	deadMansSnitchValuePresent bool, pagerDutyValuePresent bool,
) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: addonName, Namespace: pkoDeploymentNamespace}}
	err := integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, dep, "to have all replicas ready",
		func(obj client.Object) (done bool, err error) {
			deployment := obj.(*appsv1.Deployment)

			if *deployment.Spec.Replicas != deployment.Status.ReadyReplicas {
				return false, nil
			}

			deadMansSnitchOk, pagerDutyOk := false, false
			for _, envItem := range deployment.Spec.Template.Spec.Containers[0].Env {
				if envItem.Name == "MY_SNITCH_URL" {
					deadMansSnitchOk = deadMansSnitchValuePresent && deadMansSnitchUrlValue == envItem.Value || "" == envItem.Value
				} else if envItem.Name == "MY_PAGERDUTY_KEY" {
					pagerDutyOk = pagerDutyValuePresent && pagerDutyKeyValue == envItem.Value || "" == envItem.Value
				}
			}

			return deadMansSnitchOk && pagerDutyOk, nil
		})
	s.Require().NoError(err)
}

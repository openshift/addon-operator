package integration_test

import (
	"context"

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
	pkoImage               = "quay.io/alcosta/package-operator-packages/openshift/addon-operator/apnp-test-optional-params:v1.0"
	deadMansSnitchUrlValue = "https://example.com/test-snitch-url"
	pagerDutyKeyValue      = "1234567890ABCDEF"
)

func (s *integrationTestSuite) TestPackageOperatorAddon() {
	if !featuretoggle.IsEnabledOnTestEnv(&featuretoggle.AddonsPlugAndPlayFeatureToggle{}) {
		s.T().Skip("skipping PackageOperatorReconciler integration tests as the feature toggle for it is disabled in the test environment")
	}

	ctx := context.Background()

	// Addon resource
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

	// Secret resources for Dead Man's Snitch and PagerDuty, simulating the structure defined in the docs. See:
	// - https://mt-sre.github.io/docs/creating-addons/monitoring/deadmanssnitch_integration/#generated-secret
	// - https://mt-sre.github.io/docs/creating-addons/monitoring/pagerduty_integration/
	dmsSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: addonName + "-deadmanssnitch", Namespace: addonNamespace},
		Data:       map[string][]byte{"SNITCH_URL": []byte(deadMansSnitchUrlValue)},
	}
	pdSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: addonName + "-pagerduty", Namespace: addonNamespace},
		Data:       map[string][]byte{"PAGERDUTY_KEY": []byte(pagerDutyKeyValue)},
	}

	// create the Addon resource
	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)

	// wait for the Addon addonNamespace to exist (needed to publish secrets)
	err = integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: addonNamespace}}, "to be created",
		func(obj client.Object) (done bool, err error) { return true, nil })
	s.Require().NoError(err)

	// create Secrets
	err = integration.Client.Create(ctx, dmsSecret)
	s.Require().NoError(err)
	err = integration.Client.Create(ctx, pdSecret)
	s.Require().NoError(err)

	// wait until all the replicas in the Deployment inside the ClusterPackage are ready
	// and check if their env variables corresponds to the secrets
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: addonName, Namespace: "apnp-test-optional-params"}}
	err = integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, dep, "to have all replicas ready",
		validateDeployment)
	s.Require().NoError(err)

	s.T().Cleanup(func() { s.addonCleanup(addon, ctx) })
}

func validateDeployment(obj client.Object) (done bool, err error) {
	deployment := obj.(*appsv1.Deployment)

	if *deployment.Spec.Replicas != deployment.Status.ReadyReplicas {
		return false, nil
	}

	deadMansSnitchOk, pagerDutyOk := false, false
	for _, envItem := range deployment.Spec.Template.Spec.Containers[0].Env {
		if envItem.Name == "MY_SNITCH_URL" {
			deadMansSnitchOk = deadMansSnitchUrlValue == envItem.Value
		} else if envItem.Name == "MY_PAGERDUTY_KEY" {
			pagerDutyOk = pagerDutyKeyValue == envItem.Value
		}
	}

	return deadMansSnitchOk && pagerDutyOk, nil
}

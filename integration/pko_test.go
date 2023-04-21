package integration_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openshift/addon-operator/internal/controllers/addon"

	"k8s.io/apimachinery/pkg/api/meta"

	"package-operator.run/apis/core/v1alpha1"

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
	deadMansSnitchUrlValue = "https://example.com/test-snitch-url"
	pagerDutyKeyValue      = "1234567890ABCDEF"

	// source: https://github.com/kostola/package-operator-packages/tree/v1.0/openshift/addon-operator/apnp-test-optional-params
	pkoImageOptionalParams = "quay.io/alcosta/package-operator-packages/openshift/addon-operator/apnp-test-optional-params:v1.0"
	// source: https://github.com/kostola/package-operator-packages/tree/v1.0/openshift/addon-operator/apnp-test-required-params
	pkoImageRequiredParams = "quay.io/alcosta/package-operator-packages/openshift/addon-operator/apnp-test-required-params:v1.0"
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
		clusterPackageStatus       string
	}{
		{
			"OptionalParamsAllMissing", pkoImageOptionalParams,
			false, false,
			v1alpha1.PackageAvailable,
		},
		{
			"OptionalParams1stMissing", pkoImageOptionalParams,
			false, true,
			v1alpha1.PackageAvailable,
		},
		{
			"OptionalParams2ndMissing", pkoImageOptionalParams,
			true, false,
			v1alpha1.PackageAvailable,
		},
		{
			"OptionalParamsAllPresent", pkoImageOptionalParams,
			true, true,
			v1alpha1.PackageAvailable,
		},
		{
			"RequiredParamsAllMissing", pkoImageRequiredParams,
			false, false,
			v1alpha1.PackageInvalid,
		},
		{
			"RequiredParams1stMissing", pkoImageRequiredParams,
			false, true,
			v1alpha1.PackageInvalid,
		},
		{
			"RequiredParams2ndMissing", pkoImageRequiredParams,
			true, false,
			v1alpha1.PackageInvalid,
		},
		{
			"RequiredParamsAllPresent", pkoImageRequiredParams,
			true, true,
			v1alpha1.PackageAvailable,
		},
	}

	for index, test := range tests {
		s.Run(test.name, func() {
			testAddonName := fmt.Sprintf("%s-%d", addonName, index)
			testAddonNamespace := fmt.Sprintf("%s-%d", addonNamespace, index)
			ctx := context.Background()

			addon := s.createAddon(ctx, testAddonName, testAddonNamespace, test.pkoImage)
			s.waitForNamespace(ctx, testAddonNamespace)

			if test.deployDeadMansSnitchSecret {
				s.createDeadMansSnitchSecret(ctx, testAddonName, testAddonNamespace)
			}
			if test.deployPagerDutySecret {
				s.createPagerDutySecret(ctx, testAddonName, testAddonNamespace)
			}

			s.waitForClusterPackage(ctx, testAddonName, test.clusterPackageStatus, test.deployDeadMansSnitchSecret, test.deployPagerDutySecret)

			s.T().Cleanup(func() { s.addonCleanup(addon, ctx) })
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
func (s *integrationTestSuite) waitForClusterPackage(ctx context.Context, addonName string,
	conditionType string, deadMansSnitchUrlValuePresent bool, pagerDutyValuePresent bool,
) {
	cp := &v1alpha1.ClusterPackage{ObjectMeta: metav1.ObjectMeta{Name: addonName}}
	err := integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, cp, "to be available",
		clusterPackageChecker(conditionType, deadMansSnitchUrlValuePresent, pagerDutyValuePresent))
	s.Require().NoError(err)
}

func clusterPackageChecker(conditionType string, deadMansSnitchUrlValuePresent bool, pagerDutyValuePresent bool) func(client.Object) (done bool, err error) {
	if conditionType == v1alpha1.PackageInvalid {
		return func(obj client.Object) (done bool, err error) {
			clusterPackage := obj.(*v1alpha1.ClusterPackage)
			return meta.IsStatusConditionTrue(clusterPackage.Status.Conditions, conditionType), nil
		}
	}

	return func(obj client.Object) (done bool, err error) {
		clusterPackage := obj.(*v1alpha1.ClusterPackage)
		if !meta.IsStatusConditionTrue(clusterPackage.Status.Conditions, conditionType) {
			return false, nil
		}

		config := make(map[string]map[string]string)
		if err := json.Unmarshal(clusterPackage.Spec.Config.Raw, &config); err != nil {
			return false, err
		}

		addonsv1, present := config["addonsv1"]
		if !present {
			return false, nil
		}

		deadMansSnitchUrlValueOk, pagerDutyValueOk := false, false
		if deadMansSnitchUrlValuePresent {
			value, present := addonsv1[addon.DeadMansSnitchUrlConfigKey]
			deadMansSnitchUrlValueOk = present && value == deadMansSnitchUrlValue
		} else {
			_, present := addonsv1[addon.DeadMansSnitchUrlConfigKey]
			deadMansSnitchUrlValueOk = !present
		}
		if pagerDutyValuePresent {
			value, present := addonsv1[addon.PagerDutyKeyConfigKey]
			pagerDutyValueOk = present && value == pagerDutyKeyValue
		} else {
			_, present := addonsv1[addon.PagerDutyKeyConfigKey]
			pagerDutyValueOk = !present
		}

		return deadMansSnitchUrlValueOk && pagerDutyValueOk, nil
	}
}

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"
	"github.com/openshift/addon-operator/internal/controllers/addon"
	"github.com/openshift/addon-operator/internal/featuretoggle"
	"github.com/openshift/addon-operator/internal/testutil"

	"package-operator.run/apis/core/v1alpha1"
	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
)

const (
	addonName              = "pko-test"
	addonNamespace         = "pko-test-ns"
	deadMansSnitchUrlValue = "https://example.com/test-snitch-url"
	pagerDutyKeyValue      = "1234567890ABCDEF"

	// source: https://github.com/kostola/package-operator-packages/tree/v4.0/openshift/addon-operator/apnp-test-optional-params
	pkoImageOptionalParams = "quay.io/alcosta/package-operator-packages/openshift/addon-operator/apnp-test-optional-params:v4.0"
	// source: https://github.com/kostola/package-operator-packages/tree/v4.0/openshift/addon-operator/apnp-test-required-params
	pkoImageRequiredParams = "quay.io/alcosta/package-operator-packages/openshift/addon-operator/apnp-test-required-params:v4.0"
)

func (s *integrationTestSuite) TestPackageOperatorReconcilerStatusPropagatedToAddon() {
	if !featuretoggle.IsEnabledOnTestEnv(&featuretoggle.AddonsPlugAndPlayFeatureToggle{}) {
		s.T().Skip("skipping PackageOperatorReconciler integration tests as the feature toggle for it is disabled in the test environment")
	}

	ctx := context.Background()

	name := "addonname-pko-boatboat"

	image := "nonExistantImage"
	namespace := "redhat-reference-addon" // This namespace is hard coded in managed tenants bundles

	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: addonsv1alpha1.AddonSpec{
			Version:              "1.0",
			DisplayName:          name,
			AddonPackageOperator: &addonsv1alpha1.AddonPackageOperator{Image: image},
			Namespaces:           []addonsv1alpha1.AddonNamespace{{Name: namespace}},
			Install: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						Namespace:          namespace,
						CatalogSourceImage: referenceAddonCatalogSourceImageWorking,
						Channel:            "alpha",
						PackageName:        "reference-addon",
						Config:             &addonsv1alpha1.SubscriptionConfig{EnvironmentVariables: referenceAddonConfigEnvObjects},
					},
				},
			},
		},
	}

	tmpl := &pkov1alpha1.ClusterObjectTemplate{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}

	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)
	// wait until ClusterObjectTemplate is created
	err = integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, tmpl, "to be created",
		func(obj client.Object) (done bool, err error) {
			_ = obj.(*pkov1alpha1.ClusterObjectTemplate)
			return true, nil
		})
	s.Require().NoError(err)

	err = integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, addon, "to be unavailable",
		func(obj client.Object) (done bool, err error) {
			addonBrokenImage := obj.(*addonsv1alpha1.Addon)
			availableCondition := meta.FindStatusCondition(addonBrokenImage.Status.Conditions, addonsv1alpha1.Available)
			done = availableCondition.Status == metav1.ConditionFalse &&
				availableCondition.Reason == addonsv1alpha1.AddonReasonUnreadyClusterPackageTemplate
			return done, nil
		})
	s.Require().NoError(err)

	// Patch image
	patchedImage := "quay.io/osd-addons/reference-addon-package:56916cb"
	patch := fmt.Sprintf(`{"spec":{"packageOperator":{"image":"%s"}}}`, patchedImage)
	err = integration.Client.Patch(ctx, addon, client.RawPatch(types.MergePatchType, []byte(patch)))
	s.Require().NoError(err)

	// wait until ClusterObjectTemplate image is patched and is available
	err = integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, tmpl, "to be patched",
		func(obj client.Object) (done bool, err error) {
			clusterObjectTemplate := obj.(*pkov1alpha1.ClusterObjectTemplate)
			if !strings.Contains(clusterObjectTemplate.Spec.Template, patchedImage) {
				return false, nil
			}
			meta.IsStatusConditionTrue(clusterObjectTemplate.Status.Conditions, pkov1alpha1.PackageAvailable)
			return true, nil
		})
	s.Require().NoError(err)

	err = integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, addon, "to be available",
		func(obj client.Object) (done bool, err error) {
			addonAfterPatch := obj.(*addonsv1alpha1.Addon)
			availableCondition := meta.FindStatusCondition(addonAfterPatch.Status.Conditions, addonsv1alpha1.Available)
			done = availableCondition.Status == metav1.ConditionTrue
			return done, nil
		})
	s.Require().NoError(err)

	s.T().Cleanup(func() { s.addonCleanup(addon, ctx) })
}

type TestPKOSourcesData struct {
	name                        string
	resourceSuffix              string
	requiredParameters          bool
	pkoImage                    string
	deployAddonParametersSecret bool
	deployDeadMansSnitchSecret  bool
	deployPagerDutySecret       bool
	deploySendGridSecret        bool
	clusterPackageStatus        string
}

func (s *integrationTestSuite) TestPackageOperatorReconcilerSourceParameterInjection() {
	if !featuretoggle.IsEnabledOnTestEnv(&featuretoggle.AddonsPlugAndPlayFeatureToggle{}) {
		s.T().Skip("skipping PackageOperatorReconciler integration tests as the feature toggle for it is disabled in the test environment")
	}

	parValues := map[string]bool{
		"Opt": false,
		"Req": true,
	}
	apValues := map[string]bool{
		"ApN": false,
		"ApY": true,
	}
	dsValues := map[string]bool{
		"DsN": false,
		"DsY": true,
	}
	pdValues := map[string]bool{
		"PdN": false,
		"PdY": true,
	}
	sgValues := map[string]bool{
		"SgN": false,
		"SgY": true,
	}	

	// create all combinations
	var tests []TestPKOSourcesData
	for parK, parV := range parValues {
		for apK, apV := range apValues {
			for dsK, dsV := range dsValues {
				for pdK, pdV := range pdValues {
					for sgK, sgV:= range sgValues {
					pkoImage := pkoImageOptionalParams
					    if parV {
						    pkoImage = pkoImageRequiredParams
					    }

					status := v1alpha1.PackageAvailable
					if parV && (!apV || !dsV || !pdV || !sgV) {
						status = pkov1alpha1.PackageInvalid
					}

					tests = append(tests, TestPKOSourcesData{
						fmt.Sprintf("%s%s%s%s%s", parK, apK, dsK, pdK, sgK),
						fmt.Sprintf("%s-%s-%s-%s-%s", strings.ToLower(parK), strings.ToLower(apK), strings.ToLower(dsK), strings.ToLower(pdK), strings.ToLower(sgK)),
						parV, pkoImage,
						apV, dsV, pdV, sgV,
						status,
					})
				    }
			    }
		    }
	    }
    }
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].name < tests[j].name
	})

	for index, test := range tests {
		s.Run(test.name, func() {
			testAddonName := fmt.Sprintf("%s-%02d-%s", addonName, index, test.resourceSuffix)
			testAddonNamespace := fmt.Sprintf("%s-%02d-%s", addonNamespace, index, test.resourceSuffix)
			ctx := context.Background()

			addon := s.createAddon(ctx, testAddonName, testAddonNamespace, test.pkoImage)
			s.waitForNamespace(ctx, testAddonNamespace)

			if test.deployAddonParametersSecret {
				s.createAddonParametersSecret(ctx, testAddonName, testAddonNamespace)
			}
			if test.deployDeadMansSnitchSecret {
				s.createDeadMansSnitchSecret(ctx, testAddonName, testAddonNamespace)
			}
			if test.deployPagerDutySecret {
				s.createPagerDutySecret(ctx, testAddonName, testAddonNamespace)
			}
			if test.deploySendGridSecret {
				s.createSendGridSecret(ctx, testAddonName, testAddonNamespace)
			}
			s.waitForClusterPackage(
				ctx,
				testAddonName,
				testAddonNamespace,
				test.clusterPackageStatus,
				test.deployAddonParametersSecret,
				test.deployDeadMansSnitchSecret,
				test.deployPagerDutySecret,
				test.deploySendGridSecret,
			)

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

// create the Secret resource for Addon Parameters
func (s *integrationTestSuite) createAddonParametersSecret(ctx context.Context, addonName string, addonNamespace string) {
	s.createSecret(ctx, "addon-"+addonName+"-parameters", addonNamespace, map[string][]byte{"foo1": []byte("bar"), "foo2": []byte("baz")})
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
//create the Secret resource for SendGrid as defined here:
// - https://mt-sre.github.io/docs/creating-addons/monitoring/ocm_sendgrid_service_integration/
func (s *integrationTestSuite) createSendGridSecret(ctx context.Context, addonName string, addonNamespace string) {
	s.createSecret(ctx, addonName+"-smtp", addonNamespace, map[string][]byte{"host": []byte("clusterID"), "password": []byte("pwd"), "port": []byte("1111"), "tls": []byte("true"), "username": []byte("user")})
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
func (s *integrationTestSuite) waitForClusterPackage(ctx context.Context, addonName string, addonNamespace string, conditionType string,
	addonParametersValuePresent bool, deadMansSnitchUrlValuePresent bool, pagerDutyValuePresent bool, sendGridValuePresent bool,
) {
	logger := testutil.NewLogger(s.T())
	cp := &v1alpha1.ClusterPackage{ObjectMeta: metav1.ObjectMeta{Name: addonName}}
	err := integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, cp, "to be "+conditionType,
		clusterPackageChecker(&logger, addonNamespace, conditionType, addonParametersValuePresent, deadMansSnitchUrlValuePresent, pagerDutyValuePresent, sendGridValuePresent))
	s.Require().NoError(err)
}

func clusterPackageChecker(
	logger *logr.Logger,
	addonNamespace string,
	conditionType string,
	addonParametersValuePresent bool,
	deadMansSnitchUrlValuePresent bool,
	pagerDutyValuePresent bool,
	sendGridValuePresent bool,
) func(client.Object) (done bool, err error) {
	if conditionType == v1alpha1.PackageInvalid {
		return func(obj client.Object) (done bool, err error) {
			clusterPackage := obj.(*v1alpha1.ClusterPackage)
			logJson(logger, "expecting "+pkov1alpha1.PackageInvalid+" package: ", clusterPackage)
			result := meta.IsStatusConditionTrue(clusterPackage.Status.Conditions, conditionType)
			logger.Info(fmt.Sprintf("result: %t", result))
			return result, nil
		}
	}

	return func(obj client.Object) (done bool, err error) {
		clusterPackage := obj.(*v1alpha1.ClusterPackage)
		logJson(logger, "expecting "+pkov1alpha1.PackageAvailable+" package: ", clusterPackage)

		if !meta.IsStatusConditionTrue(clusterPackage.Status.Conditions, conditionType) {
			logger.Info("result: false (wrong status condition)")
			return false, nil
		}

		config := make(map[string]map[string]interface{})
		if err := json.Unmarshal(clusterPackage.Spec.Config.Raw, &config); err != nil {
			logger.Info("result: false (can't deserialize config map)")
			return false, err
		}

		logJson(logger, "config: ", config)

		addonsv1, present := config["addonsv1"]
		if !present {
			logger.Info("result: false (no 'addonsv1' key in config)")
			return false, nil
		}

		targetNamespace, present := addonsv1[addon.TargetNamespaceConfigKey]
		targetNamespaceValueOk := present && targetNamespace == addonNamespace

		clusterID, present := addonsv1[addon.ClusterIDConfigKey]
		clusterIDValueOk := false
		if present {
			_, err := uuid.Parse(fmt.Sprintf("%v", clusterID))
			clusterIDValueOk = err == nil
		}

		ocmClusterID, present := addonsv1[addon.OcmClusterIDConfigKey]
		ocmClusterIDValueOk := present && len(fmt.Sprintf("%v", ocmClusterID)) > 0

		ocmClusterName, present := addonsv1[addon.OcmClusterNameConfigKey]
		ocmClusterNameValueOk := present && len(fmt.Sprintf("%v", ocmClusterName)) > 0

		addonParametersValueOk, deadMansSnitchUrlValueOk, pagerDutyValueOk, sendGridValueOk := false, false, false, false
		if addonParametersValuePresent {
			value, present := addonsv1[addon.ParametersConfigKey]
			if present {
				jsonValue, err := json.Marshal(value)
				if err == nil {
					addonParametersValueOk = string(jsonValue) == "{\"foo1\":\"bar\",\"foo2\":\"baz\"}"
				}
			}
		} else {
			_, present := addonsv1[addon.ParametersConfigKey]
			addonParametersValueOk = !present
		}
		if deadMansSnitchUrlValuePresent {
			value, present := addonsv1[addon.DeadMansSnitchUrlConfigKey]
			deadMansSnitchUrlValueOk = present && fmt.Sprint(value) == deadMansSnitchUrlValue
		} else {
			_, present := addonsv1[addon.DeadMansSnitchUrlConfigKey]
			deadMansSnitchUrlValueOk = !present
		}
		if pagerDutyValuePresent {
			value, present := addonsv1[addon.PagerDutyKeyConfigKey]
			pagerDutyValueOk = present && fmt.Sprint(value) == pagerDutyKeyValue
		} else {
			_, present := addonsv1[addon.PagerDutyKeyConfigKey]
			pagerDutyValueOk = !present
		}
		if sendGridValuePresent {
			value, present := addonsv1[addon.SendGridConfigKey]
			if present {
				jsonValue, err := json.Marshal(value)
				if err == nil {
					sendGridValueOk = string(jsonValue) == "{\"host\":\"clusterID\",\"password\":\"pwd\",\"port\":\"1111\",\"tls\":\"true\",\"username\":\"user\"}"
				}
			}
		} else {
			_, present := addonsv1[addon.SendGridConfigKey]
			sendGridValueOk = !present
		}

		logger.Info(fmt.Sprintf("targetNamespace=%t, clusterID=%t, ocmClusterID=%t, ocmClusterName=%t, addonParameters=%t, deadMansSnitchUrl=%t, pagerDutyKey=%t, sendGrid=%t",
			targetNamespaceValueOk,
			clusterIDValueOk,
			ocmClusterIDValueOk,
			ocmClusterNameValueOk,
			addonParametersValueOk,
			deadMansSnitchUrlValueOk,
			pagerDutyValueOk,
			sendGridValueOk))

		result := targetNamespaceValueOk &&
			clusterIDValueOk &&
			ocmClusterIDValueOk &&
			ocmClusterNameValueOk &&
			addonParametersValueOk &&
			deadMansSnitchUrlValueOk &&
			pagerDutyValueOk &&
			sendGridValueOk

		logger.Info(fmt.Sprintf("result: %t", result))
		return result, nil
	}
}

func logJson(logger *logr.Logger, prefix string, input any) {
	out, err := json.Marshal(input)
	if err != nil {
		logger.Error(err, "can't serialize to JSON")
	}
	logger.Info(prefix + string(out))
}

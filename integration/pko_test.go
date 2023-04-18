package integration_test

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/addon-operator/internal/featuretoggle"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"

	pkov1alpha1 "package-operator.run/apis/core/v1alpha1"
)

func (s *integrationTestSuite) TestPackageOperatorAddon() {
	if !featuretoggle.IsEnabledOnTestEnv(&featuretoggle.AddonsPlugAndPlayFeatureToggle{}) {
		s.T().Skip("skipping PackageOperatorReconciler integration tests as the feature toggle for it is disabled in the test environment")
	}

	ctx := context.Background()

	name := "addonname-pko-boatboat"
	image := "quay.io/package-operator/test-stub-package:v1.0.0-47-g3405dde"
	namespace := "namespace-onbgdions"

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
	// wait until Addon is available
	err = integration.WaitForObject(ctx, s.T(),
		defaultAddonAvailabilityTimeout, tmpl, "to be created",
		func(obj client.Object) (done bool, err error) {
			_ = obj.(*pkov1alpha1.ClusterObjectTemplate)
			return true, nil
		})
	s.Require().NoError(err)

	addonWithStatus := &addonsv1alpha1.Addon{}
	err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addonWithStatus)
	s.Require().NoError(err)
	availableCondition := meta.FindStatusCondition(addonWithStatus.Status.Conditions, pkov1alpha1.PackageAvailable)

	s.Assert().Equal(metav1.ConditionFalse, availableCondition.Status)
	s.Assert().Equal(addonsv1alpha1.AddonReasonUnreadyClusterPackageTemplate, availableCondition.Reason)

	s.T().Cleanup(func() { s.addonCleanup(addon, ctx) })
}

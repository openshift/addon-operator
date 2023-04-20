package integration_test

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"

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

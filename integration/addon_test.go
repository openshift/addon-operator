package integration_test

import (
	"bytes"
	"context"
	"fmt"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"
)

func (s *integrationTestSuite) TestAddon() {
	ctx := context.Background()

	srcSecret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "addon-src-secret-1",
			Namespace: integration.AddonOperatorNamespace,
		},
		Data: map[string][]byte{
			corev1.BasicAuthUsernameKey: []byte("xxx"),
			corev1.BasicAuthPasswordKey: []byte("xxx"),
		},
		Type: corev1.SecretTypeBasicAuth,
	}
	err := integration.Client.Create(ctx, srcSecret1)
	s.Require().NoError(err)

	const secretPropagationDestName = "destination-secret-1"
	addon := addon_OwnNamespace()
	addon.Spec.SecretPropagation = &addonsv1alpha1.AddonSecretPropagation{
		Secrets: []addonsv1alpha1.AddonSecretPropagationReference{
			{
				SourceSecret: corev1.LocalObjectReference{
					Name: srcSecret1.Name,
				},
				DestinationSecret: corev1.LocalObjectReference{
					Name: secretPropagationDestName,
				},
			},
		},
	}

	err = integration.Client.Create(ctx, addon)
	s.Require().NoError(err)

	// wait until Addon is available
	err = integration.WaitForObject(
		s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
		func(obj client.Object) (done bool, err error) {
			a := obj.(*addonsv1alpha1.Addon)
			return meta.IsStatusConditionTrue(
				a.Status.Conditions, addonsv1alpha1.Available), nil
		})
	s.Require().NoError(err)

	err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
	s.Require().NoError(err)
	s.Assert().Equal(addon.Spec.Version, addon.Status.ObservedVersion, "addon version should be reported")

	s.Run("namespaces exist", func() {
		for _, namespace := range addon.Spec.Namespaces {
			currentNamespace := &corev1.Namespace{}
			err := integration.Client.Get(ctx, client.ObjectKey{
				Name: namespace.Name,
			}, currentNamespace)
			s.Assert().NoError(err, "could not get Namespace %s", namespace.Name)

			s.Assert().Equal(currentNamespace.Status.Phase, corev1.NamespaceActive)
		}
	})

	s.Run("secrets propagated", func() {
		for _, namespace := range addon.Spec.Namespaces {
			destSecret := &corev1.Secret{}
			key := client.ObjectKey{
				Name:      secretPropagationDestName,
				Namespace: namespace.Name,
			}
			err := integration.Client.Get(ctx, key, destSecret)
			s.Assert().NoError(err, "could not get propagated Secret %s", key)
		}
	})

	s.Run("changes to secrets are propagated", func() {
		updatedUsername := []byte("hans")

		// Update secret data
		updatedSrcSecret1 := srcSecret1.DeepCopy()
		updatedSrcSecret1.Data[corev1.BasicAuthUsernameKey] = updatedUsername
		s.Require().NoError(integration.Client.Patch(ctx, updatedSrcSecret1, client.MergeFrom(srcSecret1)))

		for _, namespace := range addon.Spec.Namespaces {
			destSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretPropagationDestName,
					Namespace: namespace.Name,
				},
			}
			err := integration.WaitForObject(
				s.T(), defaultReconcileTimeout, destSecret,
				fmt.Sprintf("Wait for destination secret %s to be updated", client.ObjectKeyFromObject(destSecret)),
				func(obj client.Object) (done bool, err error) {
					secret := obj.(*corev1.Secret)
					done = bytes.Equal(secret.Data[corev1.BasicAuthUsernameKey], updatedUsername)
					return
				})
			s.Assert().NoError(err, "secret data has not been reconciled")
		}
	})

	s.Run("catalogsource exists", func() {
		currentCatalogSource := &operatorsv1alpha1.CatalogSource{}
		err := integration.Client.Get(ctx, client.ObjectKey{
			Name:      addon.Name,
			Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
		}, currentCatalogSource)
		s.Assert().NoError(err, "could not get CatalogSource %s", addon.Name)
		s.Assert().Equal(addon.Spec.Install.OLMOwnNamespace.CatalogSourceImage, currentCatalogSource.Spec.Image)
		s.Assert().Equal(addon.Spec.DisplayName, currentCatalogSource.Spec.DisplayName)
	})

	s.Run("subscription_csv status", func() {

		subscription := &operatorsv1alpha1.Subscription{}
		{
			err := integration.Client.Get(ctx, client.ObjectKey{
				Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
				Name:      addon.Name,
			}, subscription)
			s.Require().NoError(err)

			// Force type of `operatorsv1alpha1.SubscriptionStateAtLatest` to `operatorsv1alpha1.SubscriptionState`
			// because it is an untyped string const otherwise.
			var subscriptionAtLatest operatorsv1alpha1.SubscriptionState = operatorsv1alpha1.SubscriptionStateAtLatest
			s.Assert().Equal(subscriptionAtLatest, subscription.Status.State)
			s.Assert().NotEmpty(subscription.Status.Install)
			s.Assert().Equal("reference-addon.v0.1.0", subscription.Status.CurrentCSV)
			s.Assert().Equal("reference-addon.v0.1.0", subscription.Status.InstalledCSV)
		}

		{
			csv := &operatorsv1alpha1.ClusterServiceVersion{}
			err := integration.Client.Get(ctx, client.ObjectKey{
				Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
				Name:      subscription.Status.CurrentCSV,
			}, csv)
			s.Require().NoError(err)

			s.Assert().Equal(operatorsv1alpha1.CSVPhaseSucceeded, csv.Status.Phase)
		}
	})

	s.Run("test_subscription_config", func() {

		subscription := &operatorsv1alpha1.Subscription{}

		err := integration.Client.Get(ctx, client.ObjectKey{
			Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
			Name:      addon.Name,
		}, subscription)
		s.Require().NoError(err)
		envObjectsPresent := subscription.Spec.Config.Env
		foundEnvMap := make(map[string]string)
		for _, envObj := range envObjectsPresent {
			foundEnvMap[envObj.Name] = envObj.Value
		}
		// assert that the env objects passed while creating the addon are indeed present.
		for _, passedEnvObj := range referenceAddonConfigEnvObjects {
			foundValue, found := foundEnvMap[passedEnvObj.Name]
			s.Assert().True(found, "Passed env variable not found")
			s.Assert().Equal(passedEnvObj.Value, foundValue, "Passed env variable value doesnt match with the one created")
		}
	})

	s.T().Cleanup(func() {

		s.addonCleanup(addon, ctx)

		// assert that CatalogSource is gone
		currentCatalogSource := &operatorsv1alpha1.CatalogSource{}
		err = integration.Client.Get(ctx, client.ObjectKey{
			Name:      addon.Name,
			Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
		}, currentCatalogSource)
		s.Assert().True(k8sApiErrors.IsNotFound(err), "CatalogSource not deleted: %s", currentCatalogSource.Name)

		// assert that all Namespaces are gone
		for _, namespace := range addon.Spec.Namespaces {
			currentNamespace := &corev1.Namespace{}
			err := integration.Client.Get(ctx, client.ObjectKey{
				Name: namespace.Name,
			}, currentNamespace)
			s.Assert().True(k8sApiErrors.IsNotFound(err), "Namespace not deleted: %s", namespace.Name)
		}
	})
}

func (s *integrationTestSuite) TestAddonWithAdditionalCatalogSrc() {
	ctx := context.Background()

	addon := addonWithAdditionalCatalogSource()

	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)
	// wait until Addon is available
	err = integration.WaitForObject(
		s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
		func(obj client.Object) (done bool, err error) {
			a := obj.(*addonsv1alpha1.Addon)
			return meta.IsStatusConditionTrue(
				a.Status.Conditions, addonsv1alpha1.Available), nil
		})
	s.Require().NoError(err)

	err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
	s.Require().NoError(err)
	s.Assert().Equal(addon.Spec.Version, addon.Status.ObservedVersion, "addon version should be reported")

	s.Run("test_additional_catalogsource", func() {
		catalogSourceList := &operatorsv1alpha1.CatalogSourceList{}
		err := integration.Client.List(ctx, catalogSourceList,
			client.InNamespace(addon.Spec.Install.OLMOwnNamespace.Namespace),
		)
		s.Assert().NoError(err, "could not get CatalogSource %s", addon.Name)
		s.Assert().Equal(3, len(catalogSourceList.Items))
		expectedImages := map[string]string{
			"test-1":   referenceAddonCatalogSourceImageWorking,
			"test-2":   referenceAddonCatalogSourceImageWorking,
			addon.Name: referenceAddonCatalogSourceImageWorking,
		}
		for _, ctlgSrc := range catalogSourceList.Items {
			s.Assert().Equal(expectedImages[ctlgSrc.Name], ctlgSrc.Spec.Image)
		}
	})

	s.T().Cleanup(func() {
		s.addonCleanup(addon, ctx)
	})
}

package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/integration"
	addonUtil "github.com/openshift/addon-operator/internal/controllers/addon"
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
		ctx,
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
				ctx,
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
			Name:      addonUtil.CatalogSourceName(addon),
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
				Name:      addonUtil.SubscriptionName(addon),
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
			Name:      addonUtil.SubscriptionName(addon),
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
			Name:      addonUtil.CatalogSourceName(addon),
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

func (s *integrationTestSuite) TestAddonConditions() {
	ctx := context.Background()

	addon := addonWithVersion("v0.1.0", referenceAddonCatalogSourceImageWorking)

	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)
	// wait until Addon is available
	err = integration.WaitForObject(
		ctx,
		s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
		func(obj client.Object) (done bool, err error) {
			a := obj.(*addonsv1alpha1.Addon)
			return meta.IsStatusConditionTrue(
				a.Status.Conditions, addonsv1alpha1.Available), nil
		})
	s.Require().NoError(err)

	err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
	s.Require().NoError(err)

	s.Run("test_upgrading_condition", func() {
		// Upgrade to v5
		// -------------------------------------
		updatedAddon := addonWithVersion("v0.5.0", referenceAddonCatalogSourceImageWorkingv5)
		updatedAddon.ResourceVersion = addon.ResourceVersion
		err := integration.Client.Update(ctx, updatedAddon)
		s.Require().NoError(err)

		// wait until upgrade started condition is reported as true.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, updatedAddon, "to report upgrade started condition=true",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.UpgradeStarted), nil
			},
		)
		s.Require().NoError(err)

		// Because we are upgrading, the addon should transition to available = false.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, updatedAddon, "to report available condition=false",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionFalse(
					a.Status.Conditions, addonsv1alpha1.Available), nil
			},
		)
		s.Require().NoError(err)

		// wait until upgrade succeeded condition is reported as true
		// (When the new version is available)
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, updatedAddon, "to report upgrade succeeded condition=true",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.UpgradeSucceeded), nil
			},
		)
		s.Require().NoError(err)

		err = integration.Client.Get(ctx,
			types.NamespacedName{
				Namespace: updatedAddon.Namespace,
				Name:      updatedAddon.Name,
			},
			updatedAddon)
		s.Require().NoError(err)
		// At this point the addon should be available.
		s.Require().True(meta.IsStatusConditionTrue(updatedAddon.Status.Conditions, addonsv1alpha1.Available))
		// upgrade started condition should go away
		s.Require().Nil(meta.FindStatusCondition(updatedAddon.Status.Conditions, addonsv1alpha1.UpgradeStarted))

		// ------------------------------------------------------
		// Start the upgrade to v6
		addonV6 := addonWithVersion("v0.6.0", referenceAddonCatalogSourceImageWorkingv6)
		addonV6.ResourceVersion = updatedAddon.ResourceVersion
		err = integration.Client.Update(ctx, addonV6)
		s.Require().NoError(err)

		// wait until upgrade started condition is reported as true.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addonV6, "to report upgrade started condition=true",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.UpgradeStarted), nil
			},
		)
		s.Require().NoError(err)
		// At this point, the previous upgrade succeeded status should go away.
		err = integration.Client.Get(ctx,
			types.NamespacedName{
				Namespace: addonV6.Namespace,
				Name:      addonV6.Name,
			},
			addonV6)
		s.Require().NoError(err)
		s.Require().Nil(meta.FindStatusCondition(addonV6.Status.Conditions, addonsv1alpha1.UpgradeSucceeded))
	})

	s.Run("test_installed_condition", func() {
		// remove addon before starting the test.
		s.addonCleanup(addon, ctx)

		addon := addonWithVersion("v0.1.0", referenceAddonCatalogSourceImageWorking)

		err := integration.Client.Create(ctx, addon)
		s.Require().NoError(err)

		// assert that the installed condition is present and is set to false, when the
		// addon is being installed.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to have installed condition set to false",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionFalse(
					a.Status.Conditions, addonsv1alpha1.Installed), nil
			})
		s.Require().NoError(err)

		// wait until Addon has installed=true.
		// i.e; Addon Instance has been installed & CSV phase has succeeded.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to be installed",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.Installed), nil
			})
		s.Require().NoError(err)

		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)

		// assert that the required conditions are present
		availableCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Available)
		s.Require().NotNil(availableCond)
		s.Require().Equal(metav1.ConditionTrue, availableCond.Status)
		installedCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Installed)
		s.Require().NotNil(installedCond)
		s.Require().Equal(metav1.ConditionTrue, installedCond.Status)

		// We simulate the uninstallation flow by removing the CSV and creating the delete configmap
		// in the addon's target namespace.
		CSVList := &operatorsv1alpha1.ClusterServiceVersionList{}
		addonTargetNS := addonUtil.GetCommonInstallOptions(addon).Namespace
		addonPackageName := addonUtil.GetCommonInstallOptions(addon).PackageName
		err = integration.Client.List(ctx, CSVList, client.InNamespace(addonTargetNS))
		s.Require().NoError(err)
		s.Require().NotEmpty(CSVList.Items)
		// Delete the addon's CSV
		for i := range CSVList.Items {
			currCSV := &CSVList.Items[i]
			if strings.HasPrefix(currCSV.Name, addonPackageName) {
				err = integration.Client.Delete(ctx, currCSV)
				s.Require().NoError(err)
			}
		}
		// Create the delete configmap
		deleteConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      addon.Name,
				Namespace: addonTargetNS,
				Labels: map[string]string{
					fmt.Sprintf("api.openshift.com/addon-%v-delete", addon.Name): "",
				},
			},
		}
		err = integration.Client.Create(ctx, deleteConfigMap)
		s.Require().NoError(err)
		// wait for installed=false condition to be reported.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to be uninstalled",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionFalse(
					a.Status.Conditions, addonsv1alpha1.Installed), nil
			})
		s.Require().NoError(err)
		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)
		// Assert missing CSV reason is reported.
		availableCond = meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Available)
		s.Require().NotNil(availableCond)
		s.Require().Equal(metav1.ConditionFalse, availableCond.Status)
		s.Require().Equal(addonsv1alpha1.AddonReasonMissingCSV, availableCond.Reason)
	})

	s.Run("sets_installed=true_with_install_ack_required", func() {
		// Remove addon before starting the test.
		s.addonCleanup(addon, ctx)

		addon := addonWithInstallAck()

		err := integration.Client.Create(ctx, addon)
		s.Require().NoError(err)

		addonCSV := &operatorsv1alpha1.ClusterServiceVersion{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "namespace-onbgdions",
				Name:      "reference-addon.v0.1.0",
			},
		}

		// Assert that the csv phase has succeeded while the
		// addon is being installed.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addonCSV, "to have succeeded phase",
			func(obj client.Object) (done bool, err error) {
				csv := obj.(*operatorsv1alpha1.ClusterServiceVersion)
				return csv.Status.Phase == operatorsv1alpha1.CSVPhaseSucceeded, nil
			})
		s.Require().NoError(err)

		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)

		// Assert that the addon's installed condition is false.
		// i.e. even though the csv phase has succeeded, addon doesn't report as installed.
		s.Require().True(meta.IsStatusConditionFalse(addon.Status.Conditions, addonsv1alpha1.Installed))

		instance := &addonsv1alpha1.AddonInstance{}
		err = integration.Client.Get(ctx, types.NamespacedName{
			Name:      addonsv1alpha1.DefaultAddonInstanceName,
			Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
		}, instance)
		s.Require().NoError(err)

		// Assert 'Installed' condition is not present in addon instance.
		s.Require().Nil(meta.FindStatusCondition(instance.Status.Conditions, addonsv1alpha1.AddonInstanceConditionInstalled.String()))

		// We impersonate the addon's operator and report its addon instance as installed.
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    addonsv1alpha1.AddonInstanceConditionInstalled.String(),
			Reason:  addonsv1alpha1.AddonReasonInstanceInstalled,
			Message: "Addon instance is installed",
			Status:  metav1.ConditionTrue,
		})

		err = integration.Client.Status().Update(ctx, instance)
		s.Require().NoError(err)

		// Wait until addon has installed=true.
		// i.e. Addon Instance has been installed & CSV phase has succeeded.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to be installed",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.Installed), nil
			})
		s.Require().NoError(err)

		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)

		// Assert that the required conditions are met.
		instanceInstalledCond := meta.FindStatusCondition(instance.Status.Conditions, addonsv1alpha1.AddonInstanceConditionInstalled.String())
		s.Require().NotNil(instanceInstalledCond)
		s.Require().Equal(metav1.ConditionTrue, instanceInstalledCond.Status)
		availableCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Available)
		s.Require().NotNil(availableCond)
		s.Require().Equal(metav1.ConditionTrue, availableCond.Status)
		addonInstalledCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.Installed)
		s.Require().NotNil(addonInstalledCond)
		s.Require().Equal(metav1.ConditionTrue, addonInstalledCond.Status)
	})

	s.T().Cleanup(func() {
		s.addonCleanup(addon, ctx)
	})
}

func (s *integrationTestSuite) TestAddonWithAdditionalCatalogSrc() {
	ctx := context.Background()

	addon := addonWithAdditionalCatalogSource()

	err := integration.Client.Create(ctx, addon)
	s.Require().NoError(err)
	// wait until Addon is available
	err = integration.WaitForObject(
		ctx,
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
			"test-1":                           referenceAddonCatalogSourceImageWorking,
			"test-2":                           referenceAddonCatalogSourceImageWorking,
			addonUtil.CatalogSourceName(addon): referenceAddonCatalogSourceImageWorking,
		}
		for _, ctlgSrc := range catalogSourceList.Items {
			s.Assert().Equal(expectedImages[ctlgSrc.Name], ctlgSrc.Spec.Image)
		}
	})

	s.T().Cleanup(func() {
		s.addonCleanup(addon, ctx)
	})
}

func (s *integrationTestSuite) TestAddonDeletionFlow() {
	ctx := context.Background()

	s.Run("sets readyToBeDeleted=true through addon instance strategy", func() {
		// This version of the addon doesn't respond to the delete config map.
		addon := addonWithVersion("v0.1.0", referenceAddonCatalogSourceImageWorking)
		err := integration.Client.Create(ctx, addon)
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			s.addonCleanup(addon, ctx)
		})
		// wait until Addon is available
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.Available), nil
			})
		s.Require().NoError(err)

		// Fetch the addon from the cluster

		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)

		if addon.Annotations == nil {
			addon.Annotations = map[string]string{}
		}
		addon.Annotations[addonsv1alpha1.DeleteAnnotationFlag] = "true"

		err = integration.Client.Update(ctx, addon)
		s.Require().NoError(err)

		// wait till ReadyToBeDeleted = false is reported
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to have ReadyToBeDeleted=false condition",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionFalse(
					a.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted), nil
			})
		s.Require().NoError(err)

		// Assert that the delete config map is indeed created.
		cm := &corev1.ConfigMap{}
		err = integration.Client.Get(ctx,
			types.NamespacedName{
				Name:      addon.Name,
				Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
			},
			cm,
		)
		s.Require().NoError(err)

		val, found := cm.Labels[fmt.Sprintf("api.openshift.com/addon-%v-delete", addon.Name)]
		s.Require().True(found)
		s.Require().Equal("", val)

		// Assert that the addon instance's spec.markedForDeletion is set to true.
		instance := &addonsv1alpha1.AddonInstance{}
		err = integration.Client.Get(ctx, types.NamespacedName{
			Name:      addonsv1alpha1.DefaultAddonInstanceName,
			Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
		}, instance)

		s.Require().NoError(err)
		s.Require().True(instance.Spec.MarkedForDeletion)

		// We act like the addon's operator and add the ready to deleted condition to our addon instance.

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    addonsv1alpha1.AddonInstanceConditionReadyToBeDeleted.String(),
			Reason:  addonsv1alpha1.AddonInstanceReasonReadyToBeDeleted.String(),
			Message: "Cleanup up resources.",
			Status:  metav1.ConditionTrue,
		})

		err = integration.Client.Status().Update(ctx, instance)
		s.Require().NoError(err)

		// Now we wait for readytoBeDeleted=true condition in the addonCR.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to have ReadyToBeDeleted=true condition",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted), nil
			})
		s.Require().NoError(err)
	})

	s.Run("sets readyToBeDeleted=true through legacy strategy", func() {
		// This version of the addon responds to delete config map.
		addon := addonWithVersion("v0.8.0", referenceAddonCatalogSourceImageWorkingLatest)
		// Set name to "reference-addon" as the reference-addon's operator only
		// responds if the delete CM has the name "reference-addon".
		addon.Name = "reference-addon"
		err := integration.Client.Create(ctx, addon)
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			s.addonCleanup(addon, ctx)
		})
		// wait until Addon is available
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.Available), nil
			})
		s.Require().NoError(err)

		// Fetch the addon from the cluster

		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)

		if addon.Annotations == nil {
			addon.Annotations = map[string]string{}
		}
		addon.Annotations[addonsv1alpha1.DeleteAnnotationFlag] = "true"

		err = integration.Client.Update(ctx, addon)
		s.Require().NoError(err)

		// Now we wait for readytoBeDeleted=true condition in the addonCR.(Addon has removed its CSV)
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to have ReadyToBeDeleted=true condition",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted), nil
			})
		s.Require().NoError(err)

		// Assert that the delete config map is indeed created.
		cm := &corev1.ConfigMap{}
		err = integration.Client.Get(ctx,
			types.NamespacedName{
				Name:      addon.Name,
				Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
			},
			cm,
		)
		s.Require().NoError(err)

		val, found := cm.Labels[fmt.Sprintf("api.openshift.com/addon-%v-delete", addon.Name)]
		s.Require().True(found)
		s.Require().Equal("", val)

		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)
		// addon has set installed=false.
		s.Require().True(meta.IsStatusConditionFalse(addon.Status.Conditions, addonsv1alpha1.Installed))
	})

	s.Run("addon deletion timeout", func() {
		// This version of the addon doesn't respond to the delete config map.
		addon := addonWithVersion("v0.1.0", referenceAddonCatalogSourceImageWorking)
		err := integration.Client.Create(ctx, addon)
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			s.addonCleanup(addon, ctx)
		})
		// wait until Addon is available
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.Available), nil
			})
		s.Require().NoError(err)

		// Fetch the addon from the cluster

		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)

		if addon.Annotations == nil {
			addon.Annotations = map[string]string{}
		}
		addon.Annotations[addonsv1alpha1.DeleteAnnotationFlag] = "true"
		addon.Annotations[addonsv1alpha1.DeleteTimeoutDuration] = "1m"

		err = integration.Client.Update(ctx, addon)
		s.Require().NoError(err)

		// wait till ReadyToBeDeleted = false is reported
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to have ReadyToBeDeleted=false condition",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionFalse(
					a.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted), nil
			})
		s.Require().NoError(err)

		// wait for deletetimeout condition
		err = integration.WaitForObject(
			ctx,
			s.T(), time.Minute*2, addon, "to have deletetimeout=true condition",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.DeleteTimeout), nil
			})
		s.Require().NoError(err)

		// Assert that the addon instance's spec.markedForDeletion is set to true.
		instance := &addonsv1alpha1.AddonInstance{}
		err = integration.Client.Get(ctx, types.NamespacedName{
			Name:      addonsv1alpha1.DefaultAddonInstanceName,
			Namespace: addon.Spec.Install.OLMOwnNamespace.Namespace,
		}, instance)

		s.Require().NoError(err)
		s.Require().True(instance.Spec.MarkedForDeletion)

		// We act like the addon's operator and add the ready to deleted condition to our addon instance.
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    addonsv1alpha1.AddonInstanceConditionReadyToBeDeleted.String(),
			Reason:  addonsv1alpha1.AddonInstanceReasonReadyToBeDeleted.String(),
			Message: "Cleanup up resources.",
			Status:  metav1.ConditionTrue,
		})

		err = integration.Client.Status().Update(ctx, instance)
		s.Require().NoError(err)

		// Now we wait for readytoBeDeleted=true condition in the addonCR.
		err = integration.WaitForObject(
			ctx,
			s.T(), defaultAddonAvailabilityTimeout, addon, "to have ReadyToBeDeleted=true condition",
			func(obj client.Object) (done bool, err error) {
				a := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(
					a.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted), nil
			})
		s.Require().NoError(err)

		err = integration.Client.Get(ctx, client.ObjectKeyFromObject(addon), addon)
		s.Require().NoError(err)

		// Delete timeout condition should be removed.
		timeoutCond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.DeleteTimeout)
		s.Require().Nil(timeoutCond)
	})

}

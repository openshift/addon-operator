package integration_test

import (
	"context"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	addonUtil "github.com/openshift/addon-operator/controllers/addon"
	"github.com/openshift/addon-operator/integration"
)

func (s *integrationTestSuite) TestResourceAdoption() {
	requiredOLMObjects := []client.Object{
		namespace_TestResourceAdoption(),
		catalogsource_TestResourceAdoption(),
		operatorgroup_TestResourceAdoption(),
		subscription_TestResourceAdoption(),
	}

	ctx := context.Background()
	for _, obj := range requiredOLMObjects {
		obj := obj
		s.T().Logf("creating %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName())
		err := integration.Client.Create(ctx, obj)
		s.Require().NoError(err)
	}

	addon := addon_TestResourceAdoption()

	s.Run("resource adoption", func() {
		addon := addon.DeepCopy()

		err := integration.Client.Create(ctx, addon)
		s.Require().NoError(err)

		observedAddon := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: referenceAddonName,
			},
		}

		err = integration.WaitForObject(
			ctx,
			s.T(), 10*time.Minute, observedAddon, "to be available",
			func(obj client.Object) (done bool, err error) {
				addon := obj.(*addonsv1alpha1.Addon)
				return meta.IsStatusConditionTrue(addon.Status.Conditions,
					addonsv1alpha1.Available), nil
			})
		s.Require().NoError(err)

		// validate ownerReference on Namespace
		{
			observedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: referenceAddonNamespace,
				},
			}
			err = integration.WaitForObject(
				ctx,
				s.T(), 2*time.Minute, observedNs, "to have AddonOperator ownerReference",
				func(obj client.Object) (done bool, err error) {
					ns := obj.(*corev1.Namespace)
					return validateOwnerReference(addon, ns)
				})
			s.Require().NoError(err)
		}

		// validate ownerReference on Subscription
		{
			observedSubscription := &operatorsv1alpha1.Subscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addonUtil.SubscriptionName(addon),
					Namespace: referenceAddonNamespace,
				},
			}
			err = integration.WaitForObject(
				ctx,
				s.T(), 2*time.Minute, observedSubscription, "to have AddonOperator ownerReference",
				func(obj client.Object) (done bool, err error) {
					sub := obj.(*operatorsv1alpha1.Subscription)
					return validateOwnerReference(addon, sub)
				})
			s.Require().NoError(err)

		}

		// validate ownerReference on OperatorGroup
		{
			observedOG := &operatorsv1.OperatorGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controllers.DefaultOperatorGroupName,
					Namespace: referenceAddonNamespace,
				},
			}
			err = integration.WaitForObject(
				ctx,
				s.T(), 2*time.Minute, observedOG, "to have AddonOperator ownerReference",
				func(obj client.Object) (done bool, err error) {
					og := obj.(*operatorsv1.OperatorGroup)
					return validateOwnerReference(addon, og)
				})
			s.Require().NoError(err)

		}
		// validate ownerReference on CatalogSource
		{
			observedCS := &operatorsv1alpha1.CatalogSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addonUtil.CatalogSourceName(addon),
					Namespace: referenceAddonNamespace,
				},
			}
			err = integration.WaitForObject(
				ctx,
				s.T(), 2*time.Minute, observedCS, "to have AddonOperator ownerReference",
				func(obj client.Object) (done bool, err error) {
					cs := obj.(*operatorsv1alpha1.CatalogSource)
					return validateOwnerReference(addon, cs)
				})
			s.Require().NoError(err)

		}

		s.addonCleanup(addon, ctx)
	})
}

func validateOwnerReference(addon *addonsv1alpha1.Addon, obj metav1.Object) (bool, error) {
	ownedObject := &corev1.Namespace{}
	testScheme := runtime.NewScheme()
	_ = addonsv1alpha1.AddToScheme(testScheme)
	err := controllerutil.SetControllerReference(addon, ownedObject, testScheme)
	if err != nil {
		return false, err
	}
	return controllers.HasSameController(obj, ownedObject), nil
}

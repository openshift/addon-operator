package integration_test

import (
	"context"
	"encoding/json"
	"fmt"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/integration"
)

func (s *integrationTestSuite) TestAddon_OperatorGroup() {
	addonOwnNamespace := addon_OwnNamespace()
	addonAllNamespaces := addon_AllNamespaces()

	tests := []struct {
		name            string
		targetNamespace string
		addon           *addonsv1alpha1.Addon
	}{
		{
			name:            "OwnNamespace",
			addon:           addonOwnNamespace,
			targetNamespace: addonOwnNamespace.Spec.Install.OLMOwnNamespace.Namespace,
		},
		{
			name:            "AllNamespaces",
			addon:           addonAllNamespaces,
			targetNamespace: addonAllNamespaces.Spec.Install.OLMAllNamespaces.Namespace,
		},
	}
	// for _, test := range tests {
	// 	ctx := context.Background()
	// 	s.addonCleanup(test.addon, ctx)
	// }
	// return
	for _, test := range tests {
		s.Run(test.name, func() {
			ctx := context.Background()
			addon := test.addon

			err := integration.Client.Create(ctx, addon)
			s.Require().NoError(err)

			s.T().Cleanup(func() {
				s.addonCleanup(test.addon, ctx)
			})

			err = integration.WaitForObject(
				ctx,
				s.T(), defaultAddonAvailabilityTimeout, addon, "to be Available",
				func(obj client.Object) (done bool, err error) {
					a := obj.(*addonsv1alpha1.Addon)
					eventList := &corev1.EventList{}
					if err = integration.Client.List(
						ctx,
						eventList,
						client.InNamespace(test.targetNamespace),
					); err != nil {
						fmt.Println(err)
						return
					}
					b, err := json.Marshal(eventList)
					if err != nil {
						fmt.Println(err)
						return
					}
					fmt.Printf("===eventList: \n%v\n", string(b))

					return meta.IsStatusConditionTrue(
						a.Status.Conditions, addonsv1alpha1.Available), nil
				})
			s.Require().NoError(err)

			// check that there is an OperatorGroup in the target namespace.
			operatorGroup := &operatorsv1.OperatorGroup{}
			s.Require().NoError(integration.Client.Get(ctx, client.ObjectKey{
				Name:      controllers.DefaultOperatorGroupName,
				Namespace: test.targetNamespace,
			}, operatorGroup))
		})
	}
}

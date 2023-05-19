package addon

import (
	"context"
	"errors"
	"fmt"
	"testing"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNotifyAddonLegacy(t *testing.T) {
	t.Run("creates the delete configmap if not present", func(t *testing.T) {
		client := testutil.NewClient()
		strategy := legacyDeletionStrategy{
			client:         client,
			uncachedClient: client,
		}
		addon := testutil.NewTestAddonWithCatalogSourceImage()

		// Setup mock calls.
		client.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(testutil.NewTestErrNotFound())
		client.On(
			"Create",
			mock.Anything,
			mock.Anything,
			[]ctrlclient.CreateOption(nil)).
			Return(nil)

		err := strategy.NotifyAddon(context.Background(), addon)
		require.NoError(t, err)
		client.AssertCalled(
			t,
			"Create",
			mock.Anything,
			mock.MatchedBy(func(cm *corev1.ConfigMap) bool {
				require.Equal(t, addon.Name, cm.Name)
				require.Equal(t, GetCommonInstallOptions(addon).Namespace, cm.Namespace)
				val, found := cm.Labels[fmt.Sprintf(DeleteConfigMapLabel, addon.Name)]
				return found && val == ""
			}),
			[]ctrlclient.CreateOption(nil),
		)
		client.AssertExpectations(t)
	})

	t.Run("patches the delete configmap if label is missing", func(t *testing.T) {
		client := testutil.NewClient()
		strategy := legacyDeletionStrategy{
			client:         client,
			uncachedClient: client,
		}
		addon := testutil.NewTestAddonWithCatalogSourceImage()

		existingCM := corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      addon.Name,
				Namespace: GetCommonInstallOptions(addon).Namespace,
			},
			// No delete label.
		}

		// Setup mock calls.
		client.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				cm, ok := args.Get(2).(*corev1.ConfigMap)
				require.True(t, ok)
				*cm = existingCM
			}).
			Return(nil)
		client.On(
			"Patch",
			mock.Anything,
			mock.Anything,
			mock.Anything,
			[]ctrlclient.PatchOption(nil)).
			Return(nil)

		err := strategy.NotifyAddon(context.Background(), addon)
		require.NoError(t, err)
		client.AssertCalled(
			t,
			"Patch",
			mock.Anything,
			mock.MatchedBy(func(cm *corev1.ConfigMap) bool {
				require.Equal(t, addon.Name, cm.Name)
				require.Equal(t, GetCommonInstallOptions(addon).Namespace, cm.Namespace)
				val, found := cm.Labels[fmt.Sprintf(DeleteConfigMapLabel, addon.Name)]
				return found && val == ""
			}),
			mock.Anything,
			[]ctrlclient.PatchOption(nil),
		)
		client.AssertExpectations(t)
	})

	t.Run("doesnt patch when the existing configmap is upto spec", func(t *testing.T) {
		client := testutil.NewClient()
		strategy := legacyDeletionStrategy{
			client:         client,
			uncachedClient: client,
		}
		addon := testutil.NewTestAddonWithCatalogSourceImage()

		existingCM := corev1.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      addon.Name,
				Namespace: GetCommonInstallOptions(addon).Namespace,
				Labels: map[string]string{
					fmt.Sprintf(DeleteConfigMapLabel, addon.Name): "",
				},
			},
		}

		// Setup mock calls.
		client.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				cm, ok := args.Get(2).(*corev1.ConfigMap)
				require.True(t, ok)
				*cm = existingCM
			}).
			Return(nil)

		err := strategy.NotifyAddon(context.Background(), addon)
		require.NoError(t, err)
		client.AssertNotCalled(t, "Patch")
		client.AssertExpectations(t)
	})

}

func TestAckReceivedFromAddonLegacy(t *testing.T) {
	testCases := []struct {
		operatorResource       *operatorsv1.Operator
		getOperatorResourceErr error
		subscription           *operatorsv1alpha1.Subscription
		getSubscriptionErr     error
		expectedRes            bool
		expectedErr            error
	}{
		{
			operatorResource:       nil,
			getOperatorResourceErr: testutil.NewTestErrNotFound(),
			subscription:           nil,
			getSubscriptionErr:     nil,
			expectedRes:            false,
			expectedErr:            nil,
		},
		{
			operatorResource:       nil,
			getOperatorResourceErr: errors.New("kubeapi busy"),
			subscription:           nil,
			getSubscriptionErr:     nil,
			expectedRes:            false,
			expectedErr:            errors.New("kubeapi busy"),
		},
		{
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs: []operatorsv1.RichReference{
							{
								ObjectReference: &corev1.ObjectReference{
									Kind:       "ClusterServiceVersion",
									Namespace:  "test-addon",
									Name:       "test-addon",
									APIVersion: "operators.coreos.com/v1alpha1",
								},
								Conditions: []operatorsv1.Condition{
									{
										Type:   "Succeeded",
										Status: "True",
									},
								},
							},
						},
					},
				},
			},
			getOperatorResourceErr: nil,
			subscription:           nil,
			getSubscriptionErr:     testutil.NewTestErrNotFound(),
			expectedRes:            false,
			expectedErr:            nil,
		},
		{
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs: []operatorsv1.RichReference{
							{
								ObjectReference: &corev1.ObjectReference{
									Kind:       "ClusterServiceVersion",
									Namespace:  "test-addon",
									Name:       "test-addon",
									APIVersion: "operators.coreos.com/v1alpha1",
								},
								Conditions: []operatorsv1.Condition{
									{
										Type:   "Succeeded",
										Status: "True",
									},
								},
							},
						},
					},
				},
			},
			getOperatorResourceErr: nil,
			subscription:           nil,
			getSubscriptionErr:     errors.New("kubeapi busy"),
			expectedRes:            false,
			expectedErr:            errors.New("kubeapi busy"),
		},
		// CSV is still present.
		{
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs: []operatorsv1.RichReference{
							{
								ObjectReference: &corev1.ObjectReference{
									Kind:       "ClusterServiceVersion",
									Namespace:  referenceAddonNamespace,
									Name:       referenceAddonName,
									APIVersion: "operators.coreos.com/v1alpha1",
								},
								Conditions: []operatorsv1.Condition{
									{
										Type:   "Succeeded",
										Status: "True",
									},
								},
							},
						},
					},
				},
			},
			getOperatorResourceErr: nil,
			subscription: &operatorsv1alpha1.Subscription{
				Status: operatorsv1alpha1.SubscriptionStatus{
					CurrentCSV: referenceAddonCSVName,
				},
			},
			getSubscriptionErr: nil,
			expectedRes:        false,
			expectedErr:        nil,
		},
		// Missing CSV ref in operator resource.
		{
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs:          []operatorsv1.RichReference{},
					},
				},
			},
			getOperatorResourceErr: nil,
			subscription: &operatorsv1alpha1.Subscription{
				Status: operatorsv1alpha1.SubscriptionStatus{
					CurrentCSV: referenceAddonCSVName,
				},
			},
			getSubscriptionErr: nil,
			expectedRes:        true,
			expectedErr:        nil,
		},
	}

	for _, tc := range testCases {
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: v1.ObjectMeta{
				Name: referenceAddonName,
			},
			Spec: addonsv1alpha1.AddonSpec{
				Install: addonsv1alpha1.AddonInstallSpec{
					Type: addonsv1alpha1.OLMAllNamespaces,
					OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
						AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
							Namespace:   referenceAddonNamespace,
							PackageName: referenceAddonPackageName,
						},
					},
				},
			},
		}
		client := testutil.NewClient()
		strategy := &legacyDeletionStrategy{
			client:         client,
			uncachedClient: client,
		}
		if tc.getOperatorResourceErr != nil {
			client.On(
				"Get",
				mock.Anything,
				mock.Anything,
				testutil.IsOperatorsV1OperatorPtr,
				[]ctrlclient.GetOption(nil),
			).Return(tc.getOperatorResourceErr)
		} else {
			client.On(
				"Get",
				mock.Anything,
				mock.Anything,
				testutil.IsOperatorsV1OperatorPtr,
				[]ctrlclient.GetOption(nil),
			).Run(func(args mock.Arguments) {
				operator := args.Get(2).(*operatorsv1.Operator)
				*operator = *tc.operatorResource
			}).Return(nil)
		}

		if tc.getSubscriptionErr != nil {
			client.On(
				"Get",
				mock.Anything,
				mock.Anything,
				testutil.IsOperatorsV1Alpha1SubscriptionPtr,
				[]ctrlclient.GetOption(nil),
			).Return(tc.getSubscriptionErr)
		} else {
			client.On(
				"Get",
				mock.Anything,
				mock.Anything,
				testutil.IsOperatorsV1Alpha1SubscriptionPtr,
				[]ctrlclient.GetOption(nil),
			).Run(func(args mock.Arguments) {
				subscription := args.Get(2).(*operatorsv1alpha1.Subscription)
				*subscription = *tc.subscription
			}).Return(nil)
		}
		res, err := strategy.AckReceivedFromAddon(context.Background(), addon)
		require.Equal(t, tc.expectedRes, res)
		if tc.expectedErr != nil {
			require.Error(t, err)
		}
	}
}

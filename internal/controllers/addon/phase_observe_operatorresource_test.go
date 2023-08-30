package addon

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	internalhandler "github.com/openshift/addon-operator/internal/controllers/addon/handler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/addon-operator/internal/testutil"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

const (
	referenceAddonName                 string = "reference-addon"
	referenceAddonNamespace            string = "reference-addon"
	referenceAddonPackageName          string = "reference-addon"
	referenceAddonCSVName              string = "reference-addon"
	referenceAddonOperatorResourceName string = "reference-addon"
)

func TestObserveOperatorResource(t *testing.T) {
	type Expected struct {
		Conditions []metav1.Condition
		Result     requeueResult
	}

	testCases := map[string]struct {
		operatorResource       *operatorsv1.Operator
		addonStatus            *addonsv1alpha1.AddonStatus
		addonInstanceStatus    *addonsv1alpha1.AddonInstanceStatus
		installAckRequired     bool
		deleteConfigMapPresent *bool
		expected               Expected
	}{
		"No Operator Resource Present": {
			operatorResource: &operatorsv1.Operator{},
			expected: Expected{
				Conditions: []metav1.Condition{missingCSVCondition()},
				Result:     resultRetry,
			},
			deleteConfigMapPresent: boolPtr(false),
		},
		"Phase failed": {
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs: []operatorsv1.RichReference{
							{
								ObjectReference: &corev1.ObjectReference{
									Kind:       "ClusterServiceVersion",
									Namespace:  referenceAddonNamespace,
									Name:       referenceAddonOperatorResourceName,
									APIVersion: "operators.coreos.com/v1alpha1",
								},
								Conditions: []operatorsv1.Condition{
									{
										Type:   "Succeeded",
										Status: "False",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				Conditions: []metav1.Condition{unreadyCSVCondition("failed")},
				Result:     resultRetry,
			},
		},
		"Phase succeded": {
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs: []operatorsv1.RichReference{
							{
								ObjectReference: &corev1.ObjectReference{
									Kind:       "ClusterServiceVersion",
									Namespace:  referenceAddonNamespace,
									Name:       referenceAddonOperatorResourceName,
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
			addonStatus: &addonsv1alpha1.AddonStatus{
				LastObservedAvailableCSV: "reference-addon-prev",
			},
			expected: Expected{
				Conditions: []metav1.Condition{installedCondition(metav1.ConditionTrue), availableCondition()},
				Result:     resultNil,
			},
		},
		"sets installed condition when install ack is not required": {
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs: []operatorsv1.RichReference{
							{
								ObjectReference: &corev1.ObjectReference{
									Kind:       "ClusterServiceVersion",
									Namespace:  referenceAddonNamespace,
									Name:       referenceAddonOperatorResourceName,
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
			expected: Expected{
				Conditions: []metav1.Condition{installedCondition(metav1.ConditionTrue), availableCondition()},
				Result:     resultNil,
			},
		},
		"sets installed condition when install ack is required": {
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs: []operatorsv1.RichReference{
							{
								ObjectReference: &corev1.ObjectReference{
									Kind:       "ClusterServiceVersion",
									Namespace:  referenceAddonNamespace,
									Name:       referenceAddonOperatorResourceName,
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
			installAckRequired: true,
			addonInstanceStatus: &addonsv1alpha1.AddonInstanceStatus{
				Conditions: []metav1.Condition{addonInstanceInstalledCondition()},
			},
			expected: Expected{
				Conditions: []metav1.Condition{installedCondition(metav1.ConditionTrue), availableCondition()},
				Result:     resultNil,
			},
		},
		"does not set installed condition when install ack is required and addon instance is not installed": {
			operatorResource: &operatorsv1.Operator{
				Status: operatorsv1.OperatorStatus{
					Components: &operatorsv1.Components{
						LabelSelector: nil,
						Refs: []operatorsv1.RichReference{
							{
								ObjectReference: &corev1.ObjectReference{
									Kind:       "ClusterServiceVersion",
									Namespace:  referenceAddonNamespace,
									Name:       referenceAddonOperatorResourceName,
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
			addonInstanceStatus: &addonsv1alpha1.AddonInstanceStatus{
				// Empty condition here denotes that the addon's operator is yet to mark its
				// addon instance as installed
				Conditions: []metav1.Condition{},
			},
			installAckRequired: true,
			expected: Expected{
				Conditions: []metav1.Condition{pendingAddonInstanceInstallCondition()},
				Result:     resultRetry,
			},
		},
		"sets uninstalled condition when delete config map is present and CSV is missing": {
			operatorResource: &operatorsv1.Operator{},
			addonStatus: &addonsv1alpha1.AddonStatus{
				Conditions:               []metav1.Condition{installedCondition(metav1.ConditionTrue)},
				LastObservedAvailableCSV: "reference-addon-1",
			},
			expected: Expected{
				Conditions: []metav1.Condition{installedCondition(metav1.ConditionFalse), missingCSVCondition()},
				Result:     resultStop,
			},
			deleteConfigMapPresent: boolPtr(true),
		},
		"does not set uninstalled condition when delete configmap is not present": {
			operatorResource: &operatorsv1.Operator{},
			addonStatus: &addonsv1alpha1.AddonStatus{
				Conditions:               []metav1.Condition{installedCondition(metav1.ConditionTrue)},
				LastObservedAvailableCSV: "reference-addon-1",
			},
			expected: Expected{
				// Only missing CSV condition is reported.
				Conditions: []metav1.Condition{installedCondition(metav1.ConditionTrue), missingCSVCondition()},
				Result:     resultRetry,
			},
			deleteConfigMapPresent: boolPtr(false),
		},
	}

	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			c := testutil.NewClient()
			call := c.
				On("Get",
					mock.Anything,
					mock.IsType(client.ObjectKey{}),
					testutil.IsOperatorsV1OperatorPtr,
					mock.Anything,
				)
			call = call.
				Run(func(args mock.Arguments) {
					tc.operatorResource.DeepCopyInto(args.Get(2).(*operatorsv1.Operator))
				})

			call.Return(nil)

			operatorResourceHandler := internalhandler.NewOperatorResourceHandler()
			csvKey := client.ObjectKey{
				Namespace: referenceAddonNamespace,
				Name:      referenceAddonCSVName,
			}

			r := &olmReconciler{
				client:                  c,
				uncachedClient:          c,
				scheme:                  testutil.NewTestSchemeWithAddonsv1alpha1(),
				operatorResourceHandler: operatorResourceHandler,
			}

			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
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

			if tc.installAckRequired {
				addon.Spec.InstallAckRequired = true

				c.On(
					"Get",
					mock.Anything,
					mock.Anything,
					mock.IsType(&addonsv1alpha1.AddonInstance{}),
					mock.Anything,
				).Run(func(args mock.Arguments) {
					addonInstance := args.Get(2).(*addonsv1alpha1.AddonInstance)
					addonInstance.Status = *tc.addonInstanceStatus
				}).Return(nil)
			}

			if tc.deleteConfigMapPresent != nil && *tc.deleteConfigMapPresent {
				c.On("Get",
					mock.Anything,
					mock.IsType(client.ObjectKey{}),
					testutil.IsConfigMapPtr,
					mock.Anything,
					mock.Anything,
				).Run(func(args mock.Arguments) {
					configMap := args.Get(2).(*corev1.ConfigMap)
					configMap.Name = addon.Name
					configMap.Namespace = referenceAddonNamespace
					deleteConfigMapLabel := fmt.Sprintf("api.openshift.com/addon-%v-delete", addon.Name)
					configMap.Labels = map[string]string{
						deleteConfigMapLabel: "",
					}
				}).Return(nil)
			} else if tc.deleteConfigMapPresent != nil && !*tc.deleteConfigMapPresent {
				c.On("Get",
					mock.Anything,
					mock.IsType(client.ObjectKey{}),
					testutil.IsConfigMapPtr,
					mock.Anything,
				).Return(testutil.NewTestErrNotFound())
			}

			if tc.addonStatus != nil {
				addon.Status = *tc.addonStatus
			}

			_, err := r.observeOperatorResource(context.Background(), addon, csvKey)
			require.NoError(t, err)
			res, err := r.observeOperatorResource(context.Background(), addon, csvKey)
			require.NoError(t, err)
			c.AssertExpectations(t)
			assert.Equal(t, tc.expected.Result, res)
			assertEqualConditions(t, tc.expected.Conditions, addon.Status.Conditions)
		})
	}
}

func installedCondition(value metav1.ConditionStatus) metav1.Condition {
	if value == metav1.ConditionTrue {
		return metav1.Condition{
			Type:    addonsv1alpha1.Installed,
			Status:  metav1.ConditionTrue,
			Reason:  addonsv1alpha1.AddonReasonInstalled,
			Message: "Addon has been successfully installed.",
		}
	} else {
		return metav1.Condition{
			Type:    addonsv1alpha1.Installed,
			Status:  metav1.ConditionFalse,
			Reason:  addonsv1alpha1.AddonReasonNotInstalled,
			Message: "Addon has been uninstalled.",
		}
	}
}

func addonInstanceInstalledCondition() metav1.Condition {
	return metav1.Condition{
		Type:    addonsv1alpha1.AddonInstanceConditionInstalled.String(),
		Status:  metav1.ConditionTrue,
		Reason:  addonsv1alpha1.AddonInstanceInstalledReasonSetupComplete.String(),
		Message: "Addon Instance has been successfully installed.",
	}
}

func availableCondition() metav1.Condition {
	return metav1.Condition{
		Type:    addonsv1alpha1.Available,
		Status:  metav1.ConditionTrue,
		Message: "All components are ready.",
		Reason:  addonsv1alpha1.AddonReasonFullyReconciled,
	}
}

func unreadyCSVCondition(msg string) metav1.Condition {
	return metav1.Condition{
		Type:    addonsv1alpha1.Available,
		Status:  metav1.ConditionFalse,
		Reason:  addonsv1alpha1.AddonReasonUnreadyCSV,
		Message: fmt.Sprintf("ClusterServiceVersion is not ready: %s", msg),
	}
}

func missingCSVCondition() metav1.Condition {
	return metav1.Condition{
		Type:    addonsv1alpha1.Available,
		Status:  metav1.ConditionFalse,
		Reason:  addonsv1alpha1.AddonReasonMissingCSV,
		Message: "ClusterServiceVersion is missing.",
	}
}

func pendingAddonInstanceInstallCondition() metav1.Condition {
	return metav1.Condition{
		Type:    addonsv1alpha1.Available,
		Status:  metav1.ConditionFalse,
		Reason:  addonsv1alpha1.AddonReasonInstanceNotInstalled,
		Message: "Addon instance is not yet installed.",
	}
}

func assertEqualConditions(t *testing.T, expected []metav1.Condition, actual []metav1.Condition) {
	t.Helper()

	assert.ElementsMatch(t, dropConditionTransients(expected...), dropConditionTransients(actual...))
}

func boolPtr(in bool) *bool {
	return &in
}

func dropConditionTransients(conds ...metav1.Condition) []nonTransientCondition {
	res := make([]nonTransientCondition, 0, len(conds))

	for _, c := range conds {
		res = append(res, nonTransientCondition{
			Type:    c.Type,
			Status:  c.Status,
			Reason:  c.Reason,
			Message: c.Message,
		})
	}

	return res
}

type nonTransientCondition struct {
	Type    string
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

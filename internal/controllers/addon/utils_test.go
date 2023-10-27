package addon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	ctrl "sigs.k8s.io/controller-runtime"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestHandleAddonDeletion(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		addonToDelete := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers: []string{
					cacheFinalizer,
				},
			},
		}

		c := testutil.NewClient()

		operatorResourceHandlerMock := &operatorResourceHandlerMock{}
		r := &AddonReconciler{
			Client:                  c,
			Log:                     testutil.NewLogger(t),
			Scheme:                  testutil.NewTestSchemeWithAddonsv1alpha1(),
			operatorResourceHandler: operatorResourceHandlerMock,
		}

		c.StatusMock.
			On("Update", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		c.
			On("Update", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		c.
			On("Delete", mock.Anything, mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"), mock.Anything).
			Return(errors.NewNotFound(schema.GroupResource{}, ""))
		operatorResourceHandlerMock.
			On("Free", addonToDelete)

		ctx := context.Background()
		res, err := r.handleAddonCRDeletion(ctx, addonToDelete)
		require.NoError(t, err)
		require.True(t, res.IsZero())

		assert.Empty(t, addonToDelete.Finalizers)                                    // finalizer is gone
		assert.Equal(t, addonsv1alpha1.PhaseTerminating, addonToDelete.Status.Phase) // status is set

		// Methods have been called
		c.AssertExpectations(t)
		c.StatusMock.AssertExpectations(t)

		// test addon status condition
		availableCond := meta.FindStatusCondition(addonToDelete.Status.Conditions, addonsv1alpha1.Available)
		if assert.NotNil(t, availableCond) {
			assert.Equal(t, metav1.ConditionFalse, availableCond.Status)
			assert.Equal(t, addonsv1alpha1.AddonReasonTerminating, availableCond.Reason)
		}
	})

	t.Run("noop if finalizer already gone", func(t *testing.T) {
		addonToDelete := &addonsv1alpha1.Addon{}

		c := testutil.NewClient()

		csvEventHandlerMock := &operatorResourceHandlerMock{}
		r := &AddonReconciler{
			Client:                  c,
			Log:                     testutil.NewLogger(t),
			Scheme:                  testutil.NewTestSchemeWithAddonsv1alpha1(),
			operatorResourceHandler: csvEventHandlerMock,
		}

		c.
			On("Update", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		csvEventHandlerMock.
			On("Free", addonToDelete)

		ctx := context.Background()
		res, err := r.handleAddonCRDeletion(ctx, addonToDelete)
		require.NoError(t, err)
		require.True(t, res.IsZero())

		// ensure no API calls are made,
		// because the object is already deleted.
		c.AssertNotCalled(
			t, "Update", mock.Anything, mock.Anything, mock.Anything)
	})
}

type operatorResourceHandlerMock struct {
	mock.Mock
}

var _ operatorResourceHandler = (*operatorResourceHandlerMock)(nil)

// Create is called in response to an create event - e.g. Pod Creation.
func (m *operatorResourceHandlerMock) Create(_ context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	m.Called(e, q)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (m *operatorResourceHandlerMock) Update(_ context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	m.Called(e, q)
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (m *operatorResourceHandlerMock) Delete(_ context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	m.Called(e, q)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (m *operatorResourceHandlerMock) Generic(_ context.Context, e event.GenericEvent, q workqueue.RateLimitingInterface) {
	m.Called(e, q)
}

func (m *operatorResourceHandlerMock) Free(addon *addonsv1alpha1.Addon) {
	m.Called(addon)
}

func (m *operatorResourceHandlerMock) UpdateMap(
	addon *addonsv1alpha1.Addon, operartorKey client.ObjectKey,
) (changed bool) {
	args := m.Called(addon, operartorKey)
	return args.Bool(0)
}

func TestParseAddonInstallConfigurationForAdditionalCatalogSources(t *testing.T) {
	// expected outcome struct for every function call
	type Expected struct {
		additionalCatalogSource []addonsv1alpha1.AdditionalCatalogSource
		targetNamespace         string
		pullSecretName          string
		stop                    bool
	}

	// all types of synthetic testcases
	testCases := []struct {
		addon    *addonsv1alpha1.Addon
		expected Expected
	}{
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:      "test-namespace-OLMOwnNamespace",
								PullSecretName: "test-pullSecretName",
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "image-1",
									},
									{
										Name:  "test-2",
										Image: "image-2",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{
					{
						Name:  "test-1",
						Image: "image-1",
					},
					{
						Name:  "test-2",
						Image: "image-2",
					},
				},
				targetNamespace: "test-namespace-OLMOwnNamespace",
				pullSecretName:  "test-pullSecretName",
				stop:            false,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:      "test-namespace-OLMOwnNamespace",
								PullSecretName: "test-pullSecretName",
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "image-1",
									},
									{
										Name:  "",
										Image: "image-2",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{},
				targetNamespace:         "",
				pullSecretName:          "",
				stop:                    true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:      "test-namespace-OLMOwnNamespace",
								PullSecretName: "test-pullSecretName",
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "image-1",
									},
									{
										Name:  "test-2",
										Image: "",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{},
				targetNamespace:         "",
				pullSecretName:          "",
				stop:                    true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:      "test-namespace-OLMOwnNamespace",
								PullSecretName: "test-pullSecretName",
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "image-1",
									},
									{
										Name:  "",
										Image: "",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{},
				targetNamespace:         "",
				pullSecretName:          "",
				stop:                    true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:      "test-namespace-OLMAllNamespaces",
								PullSecretName: "test-pullSecretName",
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "image-1",
									},
									{
										Name:  "test-2",
										Image: "image-2",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{
					{
						Name:  "test-1",
						Image: "image-1",
					},
					{
						Name:  "test-2",
						Image: "image-2",
					},
				},
				targetNamespace: "test-namespace-OLMAllNamespaces",
				pullSecretName:  "test-pullSecretName",
				stop:            false,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:      "test-namespace-OLMAllNamespaces",
								PullSecretName: "test-pullSecretName",
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "image-1",
									},
									{
										Name:  "",
										Image: "image-2",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{},
				targetNamespace:         "",
				pullSecretName:          "",
				stop:                    true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:      "test-namespace-OLMAllNamespaces",
								PullSecretName: "test-pullSecretName",
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "image-1",
									},
									{
										Name:  "test-2",
										Image: "",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{},
				targetNamespace:         "",
				pullSecretName:          "",
				stop:                    true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:      "test-namespace-OLMAllNamespaces",
								PullSecretName: "test-pullSecretName",
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "image-1",
									},
									{
										Name:  "",
										Image: "",
									},
								},
							},
						},
					},
				},
			},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{},
				targetNamespace:         "",
				pullSecretName:          "",
				stop:                    true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{},
			expected: Expected{
				additionalCatalogSource: []addonsv1alpha1.AdditionalCatalogSource{},
				targetNamespace:         "",
				pullSecretName:          "",
				stop:                    true,
			},
		},
	}
	log := controllers.LoggerFromContext(context.TODO())
	for _, tc := range testCases {
		t.Run("parse addon install configuration for additional catalogsource test", func(t *testing.T) {
			addon := tc.addon.DeepCopy()
			additionalCatalogSource, targetNamespace, pullSecretName, stop := parseAddonInstallConfigForAdditionalCatalogSources(log, addon)
			// additionalCatalogSource check
			assert.Equal(t, tc.expected.additionalCatalogSource, additionalCatalogSource)
			// targetNamespace check
			assert.Equal(t, tc.expected.targetNamespace, targetNamespace)
			// pullSecretName check
			assert.Equal(t, tc.expected.pullSecretName, pullSecretName)
			// stop check
			assert.Equal(t, tc.expected.stop, stop)
		})
	}
}

func TestParseAddonInstallConfig(t *testing.T) {
	// expected outcome struct for every function call
	type Expected struct {
		common *addonsv1alpha1.AddonInstallOLMCommon
		stop   bool
	}

	// all types of synthetic testcases
	testCases := []struct {
		addon    *addonsv1alpha1.Addon
		expected Expected
	}{
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:          "test",
								CatalogSourceImage: "test",
							},
						},
					},
				},
			},
			expected: Expected{
				common: &addonsv1alpha1.AddonInstallOLMCommon{
					Namespace:          "test",
					CatalogSourceImage: "test",
				},
				stop: false,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
					},
				},
			},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:          "",
								CatalogSourceImage: "test",
							},
						},
					},
				},
			},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:          "test",
								CatalogSourceImage: "",
							},
						},
					},
				},
			},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:          "",
								CatalogSourceImage: "",
							},
						},
					},
				},
			},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:          "test",
								CatalogSourceImage: "test",
							},
						},
					},
				},
			},
			expected: Expected{
				common: &addonsv1alpha1.AddonInstallOLMCommon{
					Namespace:          "test",
					CatalogSourceImage: "test",
				},
				stop: false,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
					},
				},
			},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:          "test",
								CatalogSourceImage: "",
							},
						},
					},
				},
			},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:          "",
								CatalogSourceImage: "test",
							},
						},
					},
				},
			},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								Namespace:          "",
								CatalogSourceImage: "",
							},
						},
					},
				},
			},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
		{
			addon: &addonsv1alpha1.Addon{},
			expected: Expected{
				common: nil,
				stop:   true,
			},
		},
	}
	log := controllers.LoggerFromContext(context.TODO())
	for _, tc := range testCases {
		t.Run("parse addon install configuration test", func(t *testing.T) {
			addon := tc.addon.DeepCopy()
			common, stop := parseAddonInstallConfig(log, addon)
			// addon install OLMCommon check
			assert.Equal(t, tc.expected.common, common)
			// stop check
			assert.Equal(t, tc.expected.stop, stop)
		})
	}
}

func TestHasAdditionalCatalogSources(t *testing.T) {
	testCases := []struct {
		addon    *addonsv1alpha1.Addon
		expected bool
	}{
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "test-1",
									},
									{
										Name:  "test-2",
										Image: "test-2",
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name:  "test-1",
										Image: "test-1",
									},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			addon:    &addonsv1alpha1.Addon{},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run("additional catalogsources test", func(t *testing.T) {
			addon := tc.addon.DeepCopy()
			result := HasAdditionalCatalogSources(addon)
			// additional catalog source check
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestHasMonitoringFederation(t *testing.T) {
	testCases := []struct {
		addon    *addonsv1alpha1.Addon
		expected bool
	}{
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Monitoring: &addonsv1alpha1.MonitoringSpec{
						Federation: &addonsv1alpha1.MonitoringFederationSpec{
							Namespace: "test",
							MatchNames: []string{
								"test",
							},
							MatchLabels: map[string]string{
								"test": "test",
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Monitoring: nil,
				},
			},
			expected: false,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Monitoring: &addonsv1alpha1.MonitoringSpec{
						Federation: nil,
					},
				},
			},
			expected: false,
		},
		{
			addon:    &addonsv1alpha1.Addon{},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run("monitoring federation test", func(t *testing.T) {
			addon := tc.addon.DeepCopy()
			result := HasMonitoringFederation(addon)
			// monitoring federation check
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestHasMonitoringStack(t *testing.T) {
	testCases := []struct {
		name     string
		addon    *addonsv1alpha1.Addon
		expected bool
	}{
		{
			name: "addon with monitoring stack defined",
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Monitoring: &addonsv1alpha1.MonitoringSpec{
						MonitoringStack: &addonsv1alpha1.MonitoringStackSpec{
							RHOBSRemoteWriteConfig: &addonsv1alpha1.RHOBSRemoteWriteConfigSpec{
								URL:       "test/url",
								Allowlist: []string{"test", "foo", "bar"},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "addon with nil monitoring",
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Monitoring: nil,
				},
			},
			expected: false,
		},
		{
			name: "addon with nil monitoring stack",
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Monitoring: &addonsv1alpha1.MonitoringSpec{
						MonitoringStack: nil,
					},
				},
			},
			expected: false,
		},
		{
			name:     "addon with nil spec",
			addon:    &addonsv1alpha1.Addon{},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addon := tc.addon.DeepCopy()
			result := HasMonitoringStack(addon)
			// monitoring stack check
			assert.Equal(t, tc.expected, result)
		})
	}
}

// The TestReportLastOvservedAvailableCSV function ensures an addon's
// LastObservedAvailableCSV field is set to the expected value.
func TestReportLastObservedAvailableCSV(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		Status: addonsv1alpha1.AddonStatus{},
	}

	// Call the function under test
	reportLastObservedAvailableCSV(addon, "test-csv")

	// Assert that the LastObservedAvailableCSV field has been updated correctly
	assert.Equal(t, "test-csv", addon.Status.LastObservedAvailableCSV)
}

// The TestReportAddonPauseStatus tests the behavior of the
// reportAddonPauseStatus function.
func TestReportAddonPauseStatus(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       addonsv1alpha1.AddonSpec{},
		Status:     addonsv1alpha1.AddonStatus{},
	}

	// Call the reportAddonPauseStatus function
	reason := "a reason"
	reportAddonPauseStatus(addon, reason)

	// Assert the expected status condition
	expectedCondition := metav1.Condition{
		Type:               addonsv1alpha1.Paused,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            "",
		ObservedGeneration: addon.Generation,
	}

	// Assert the expected condition is present
	var found bool
	for _, condition := range addon.Status.Conditions {
		if condition.Type == expectedCondition.Type &&
			condition.Status == expectedCondition.Status &&
			condition.Reason == expectedCondition.Reason &&
			condition.Message == expectedCondition.Message &&
			condition.ObservedGeneration == expectedCondition.ObservedGeneration {
			found = true
			break
		}
	}

	assert.True(t, found, "Expected condition not found")

	// Assert the expected observed generation
	assert.Equal(t, addon.Generation, addon.Status.ObservedGeneration)
}

// TestHandleExit tests the behavior of the handleExit function by
// checking its return value under different conditions.
func TestHandleExit(t *testing.T) {
	t.Run("Retry result", func(t *testing.T) {
		expectedResult := ctrl.Result{
			RequeueAfter: defaultRetryAfterTime,
		}
		result := handleExit(resultRetry)
		assert.Equal(t, expectedResult, result, "Expected %v, but got %v", expectedResult, result)
	})

	t.Run("Other result", func(t *testing.T) {
		expectedResult := ctrl.Result{}
		result := handleExit(resultStop)
		assert.Equal(t, expectedResult, result, "Expected %v, but got %v", expectedResult, result)
	})

	t.Run("Result nil", func(t *testing.T) {
		expectedResult := ctrl.Result{}
		result := handleExit(resultNil)
		assert.Equal(t, expectedResult, result, "Expected %v, but got %v", expectedResult, result)
	})
}
func TestAddonReconciler_ensureClusterPackageDeletion(t *testing.T) {
	c := testutil.NewClient()

	c.
		On("Delete",
			mock.Anything,
			mock.AnythingOfType("*v1alpha1.ClusterObjectTemplate"),
			mock.Anything,
		).
		Return(nil)

	r := &AddonReconciler{
		Client: c,
	}
	addon := &addonsv1alpha1.Addon{}
	addon.SetName("a-testtest")
	ctx := context.Background()
	res, err := r.ensureClusterPackageDeletion(ctx, addon)
	require.NoError(t, err)
	assert.Equal(t, defaultRetryAfterTime, res.RequeueAfter)
}

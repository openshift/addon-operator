package addon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
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

		csvEventHandlerMock := &csvEventHandlerMock{}
		r := &AddonReconciler{
			Client:          c,
			Log:             testutil.NewLogger(t),
			Scheme:          testutil.NewTestSchemeWithAddonsv1alpha1(),
			csvEventHandler: csvEventHandlerMock,
		}

		c.
			On("Update", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		csvEventHandlerMock.
			On("Free", addonToDelete)

		ctx := context.Background()
		err := r.handleAddonDeletion(ctx, addonToDelete)
		require.NoError(t, err)

		assert.Empty(t, addonToDelete.Finalizers)                                    // finalizer is gone
		assert.Equal(t, addonsv1alpha1.PhaseTerminating, addonToDelete.Status.Phase) // status is set

		// Methods have been called
		c.AssertExpectations(t)

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

		csvEventHandlerMock := &csvEventHandlerMock{}
		r := &AddonReconciler{
			Client:          c,
			Log:             testutil.NewLogger(t),
			Scheme:          testutil.NewTestSchemeWithAddonsv1alpha1(),
			csvEventHandler: csvEventHandlerMock,
		}

		c.
			On("Update", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		csvEventHandlerMock.
			On("Free", addonToDelete)

		ctx := context.Background()
		err := r.handleAddonDeletion(ctx, addonToDelete)
		require.NoError(t, err)

		// ensure no API calls are made,
		// because the object is already deleted.
		c.AssertNotCalled(
			t, "Update", mock.Anything, mock.Anything, mock.Anything)
	})
}

type csvEventHandlerMock struct {
	mock.Mock
}

var _ csvEventHandler = (*csvEventHandlerMock)(nil)

// Create is called in response to an create event - e.g. Pod Creation.
func (m *csvEventHandlerMock) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	m.Called(e, q)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (m *csvEventHandlerMock) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	m.Called(e, q)
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (m *csvEventHandlerMock) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	m.Called(e, q)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (m *csvEventHandlerMock) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	m.Called(e, q)
}

func (m *csvEventHandlerMock) Free(addon *addonsv1alpha1.Addon) {
	m.Called(addon)
}

func (m *csvEventHandlerMock) ReplaceMap(
	addon *addonsv1alpha1.Addon, csvKeys ...client.ObjectKey,
) (changed bool) {
	args := m.Called(addon, csvKeys)
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

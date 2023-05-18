package addon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureAddonInstance(t *testing.T) {
	t.Run("ensures AddonInstance", func(t *testing.T) {
		existingInstance := &addonsv1alpha1.AddonInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      addonsv1alpha1.DefaultAddonInstanceName,
				Namespace: "addon-system",
			},
			Spec: addonsv1alpha1.AddonInstanceSpec{
				HeartbeatUpdatePeriod: metav1.Duration{
					Duration: time.Hour,
				},
			},
		}

		addonOwnNamespace := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: "addon-1",
			},
			Spec: addonsv1alpha1.AddonSpec{
				Install: addonsv1alpha1.AddonInstallSpec{
					Type: addonsv1alpha1.OLMOwnNamespace,
					OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
						AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
							CatalogSourceImage: "quay.io/osd-addons/test:sha256:04864220677b2ed6244f2e0d421166df908986700647595ffdb6fd9ca4e5098a",
							Namespace:          "addon-system",
						},
					},
				},
			},
		}

		addonAllNamespaces := &addonsv1alpha1.Addon{
			ObjectMeta: metav1.ObjectMeta{
				Name: "addon-1",
			},
			Spec: addonsv1alpha1.AddonSpec{
				Install: addonsv1alpha1.AddonInstallSpec{
					Type: addonsv1alpha1.OLMAllNamespaces,
					OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
						AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
							CatalogSourceImage: "quay.io/osd-addons/test:sha256:04864220677b2ed6244f2e0d421166df908986700647595ffdb6fd9ca4e5098a",
							Namespace:          "addon-system",
						},
					},
				},
			},
		}

		tests := []struct {
			name                  string
			addon                 *addonsv1alpha1.Addon
			targetNamespace       string
			existingAddonInstance *addonsv1alpha1.AddonInstance
		}{
			{
				name:                  "OwnNamespace",
				addon:                 addonOwnNamespace,
				targetNamespace:       addonOwnNamespace.Spec.Install.OLMOwnNamespace.Namespace,
				existingAddonInstance: nil,
			},
			{
				name:                  "OwnNamespace",
				addon:                 addonAllNamespaces,
				targetNamespace:       addonAllNamespaces.Spec.Install.OLMAllNamespaces.Namespace,
				existingAddonInstance: existingInstance,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				log := testutil.NewLogger(t)
				c := testutil.NewClient()
				r := addonInstanceReconciler{
					client: c,
					scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
				}
				addon := test.addon

				// Mock Setup
				if test.existingAddonInstance != nil {
					c.On(
						"Get",
						mock.Anything,
						mock.Anything,
						mock.IsType(&addonsv1alpha1.AddonInstance{}),
						mock.Anything,
					).Run(func(args mock.Arguments) {
						instance := args.Get(2).(*addonsv1alpha1.AddonInstance)
						*instance = *test.existingAddonInstance
					}).Return(nil)
				} else {
					c.On(
						"Get",
						mock.Anything,
						mock.Anything,
						mock.IsType(&addonsv1alpha1.AddonInstance{}),
						mock.Anything,
					).Return(testutil.NewTestErrNotFound())
				}

				var reconciledInstance *addonsv1alpha1.AddonInstance
				// update path
				if test.existingAddonInstance != nil {
					c.
						On(
							"Update",
							mock.Anything,
							mock.IsType(&addonsv1alpha1.AddonInstance{}),
							mock.Anything,
						).
						Run(func(args mock.Arguments) {
							reconciledInstance = args.Get(1).(*addonsv1alpha1.AddonInstance)
						}).
						Return(nil)

				} else {
					// Create path
					c.
						On(
							"Create",
							mock.Anything,
							mock.IsType(&addonsv1alpha1.AddonInstance{}),
							mock.Anything,
						).
						Run(func(args mock.Arguments) {
							reconciledInstance = args.Get(1).(*addonsv1alpha1.AddonInstance)
						}).
						Return(nil)
				}
				// Test
				ctx := context.Background()
				controllers.ContextWithLogger(ctx, log)
				err := r.ensureAddonInstance(ctx, addon)
				require.NoError(t, err)

				assert.Equal(t, addonsv1alpha1.DefaultAddonInstanceName, reconciledInstance.Name)
				assert.Equal(t, test.targetNamespace, reconciledInstance.Namespace)
				assert.Equal(t,
					metav1.Duration{
						Duration: addonsv1alpha1.DefaultAddonInstanceHeartbeatUpdatePeriod,
					},
					reconciledInstance.Spec.HeartbeatUpdatePeriod,
				)
			})
		}
	})

	t.Run("gracefully handles invalid configuration", func(t *testing.T) {
		tests := []struct {
			name  string
			addon *addonsv1alpha1.Addon
		}{
			{
				name: "install.type is unsupported",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type: addonsv1alpha1.AddonInstallType("random"),
						},
					},
				},
			},
			{
				name: "ownNamespace is nil",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type: addonsv1alpha1.OLMOwnNamespace,
						},
					},
				},
			},
			{
				name: "ownNamespace.namespace is empty",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type:            addonsv1alpha1.OLMOwnNamespace,
							OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{},
						},
					},
				},
			},
			{
				name: "allNamespaces is nil",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type: addonsv1alpha1.OLMAllNamespaces,
						},
					},
				},
			},
			{
				name: "allNamespaces.namespace is empty",
				addon: &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: "addon-1",
					},
					Spec: addonsv1alpha1.AddonSpec{
						Install: addonsv1alpha1.AddonInstallSpec{
							Type:             addonsv1alpha1.OLMAllNamespaces,
							OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{},
						},
					},
				},
			},
		}

		for i, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				log := testutil.NewLogger(t)
				c := testutil.NewClient()
				r := addonInstanceReconciler{
					client: c,
					scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
				}

				// Test
				ctx := context.Background()
				controllers.ContextWithLogger(ctx, log)
				err := r.ensureAddonInstance(ctx, test.addon)
				require.EqualError(t, err, "failed to create addonInstance due to misconfigured install.spec.type")

				// check Addon Status
				// skip the first test case
				if i > 0 {
					availableCond := meta.FindStatusCondition(test.addon.Status.Conditions, addonsv1alpha1.Available)
					if assert.NotNil(t, availableCond) {
						assert.Equal(t, metav1.ConditionFalse, availableCond.Status)
						assert.Equal(t, addonsv1alpha1.AddonReasonConfigError, availableCond.Reason)
					}
				}

			})
		}
	})
}

func TestReconcileAddonInstance(t *testing.T) {
	addonInstance := &addonsv1alpha1.AddonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addonsv1alpha1.DefaultAddonInstanceName,
			Namespace: "test",
		},
		Spec: addonsv1alpha1.AddonInstanceSpec{
			HeartbeatUpdatePeriod: metav1.Duration{
				Duration: addonsv1alpha1.DefaultAddonInstanceHeartbeatUpdatePeriod,
			},
		},
	}

	t.Run("no addoninstance", func(t *testing.T) {
		c := testutil.NewClient()
		r := addonInstanceReconciler{
			client: c,
			scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		}

		c.
			On(
				"Get",
				mock.Anything,
				client.ObjectKeyFromObject(addonInstance),
				mock.IsType(&addonsv1alpha1.AddonInstance{}),
				mock.Anything,
			).
			Run(func(args mock.Arguments) {
				fetchedAddonInstance := args.Get(2).(*addonsv1alpha1.AddonInstance)
				addonInstance.DeepCopyInto(fetchedAddonInstance)
			}).
			Return(nil)

		ctx := context.Background()
		err := r.reconcileAddonInstance(ctx, addonInstance.DeepCopy())
		require.NoError(t, err)
	})

	t.Run("update", func(t *testing.T) {
		c := testutil.NewClient()
		r := addonInstanceReconciler{
			client: c,
			scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		}

		c.
			On(
				"Get",
				mock.Anything,
				client.ObjectKeyFromObject(addonInstance),
				mock.IsType(&addonsv1alpha1.AddonInstance{}),
				mock.Anything,
			).
			Return(nil)

		c.
			On(
				"Update",
				mock.Anything,
				mock.IsType(&addonsv1alpha1.AddonInstance{}),
				mock.Anything,
			).
			Return(nil)

		ctx := context.Background()
		err := r.reconcileAddonInstance(ctx, addonInstance.DeepCopy())
		require.NoError(t, err)

		c.AssertCalled(t,
			"Update",
			mock.Anything,
			mock.IsType(&addonsv1alpha1.AddonInstance{}),
			mock.Anything,
		)
	})
}

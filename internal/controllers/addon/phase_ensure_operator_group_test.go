package addon

import (
	"context"
	"testing"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureOperatorGroup(t *testing.T) {
	addonOwnNamespace := testutil.NewTestAddonWithoutNamespace()
	addonOwnNamespace.Spec.Install = addonsv1alpha1.AddonInstallSpec{
		Type: addonsv1alpha1.OLMOwnNamespace,
		OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
			AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
				CatalogSourceImage: "quay.io/osd-addons/test:sha256:04864220677b2ed6244f2e0d421166df908986700647595ffdb6fd9ca4e5098a",
				Namespace:          "addon-system",
			},
		},
	}

	addonAllNamespaces := testutil.NewTestAddonWithoutNamespace()
	addonAllNamespaces.Spec.Install = addonsv1alpha1.AddonInstallSpec{
		Type: addonsv1alpha1.OLMAllNamespaces,
		OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
			AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
				CatalogSourceImage: "quay.io/osd-addons/test:sha256:04864220677b2ed6244f2e0d421166df908986700647595ffdb6fd9ca4e5098a",
				Namespace:          "addon-system",
			},
		},
	}

	tests := []struct {
		name                     string
		addon                    *addonsv1alpha1.Addon
		targetNamespace          string
		expectedTargetNamespaces []string
	}{
		{
			name:                     "addon with OlmOwnNamespace has only its namespace in target namespaces",
			addon:                    addonOwnNamespace,
			targetNamespace:          addonOwnNamespace.Spec.Install.OLMOwnNamespace.Namespace,
			expectedTargetNamespaces: []string{addonOwnNamespace.Spec.Install.OLMOwnNamespace.Namespace},
		},
		{
			name:            "addon with OLMAllNamespaces has empty target namespaces",
			addon:           addonAllNamespaces,
			targetNamespace: addonAllNamespaces.Spec.Install.OLMAllNamespaces.Namespace,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			log := testutil.NewLogger(t)
			c := testutil.NewClient()
			r := &olmReconciler{
				client: c,
				scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
			}
			addon := test.addon

			// Mock Setup
			c.
				On(
					"Get",
					mock.Anything,
					client.ObjectKey{
						Name:      controllers.DefaultOperatorGroupName,
						Namespace: test.targetNamespace,
					},
					mock.IsType(&operatorsv1.OperatorGroup{}),
					mock.Anything,
				).
				Return(errors.NewNotFound(schema.GroupResource{}, ""))
			var createdOperatorGroup *operatorsv1.OperatorGroup
			c.
				On(
					"Create",
					mock.Anything,
					mock.IsType(&operatorsv1.OperatorGroup{}),
					mock.Anything,
				).
				Run(func(args mock.Arguments) {
					createdOperatorGroup = args.Get(1).(*operatorsv1.OperatorGroup)
				}).
				Return(nil)

			// Test
			ctx := controllers.ContextWithLogger(context.Background(), log)
			requeueResult, err := r.ensureOperatorGroup(ctx, addon)
			require.NoError(t, err)
			assert.Equal(t, resultNil, requeueResult)

			if c.AssertCalled(
				t, "Create",
				mock.Anything,
				mock.IsType(&operatorsv1.OperatorGroup{}),
				mock.Anything,
			) {
				assert.Equal(t, controllers.DefaultOperatorGroupName, createdOperatorGroup.Name)
				assert.Equal(t, test.targetNamespace, createdOperatorGroup.Namespace)
				assert.Equal(t, test.expectedTargetNamespaces, createdOperatorGroup.Spec.TargetNamespaces)
			}
		})
	}

	t.Run("guards against invalid configuration", func(t *testing.T) {
		addonOwnNamespaceIsNil := testutil.NewTestAddonWithoutNamespace()
		addonOwnNamespaceIsNil.Spec.Install = addonsv1alpha1.AddonInstallSpec{
			Type: addonsv1alpha1.OLMOwnNamespace,
		}

		addonOwnNamespaceIsEmpty := testutil.NewTestAddonWithoutNamespace()
		addonOwnNamespaceIsEmpty.Spec.Install = addonsv1alpha1.AddonInstallSpec{
			Type:            addonsv1alpha1.OLMOwnNamespace,
			OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{},
		}

		addonAllNamespacesIsNil := testutil.NewTestAddonWithoutNamespace()
		addonAllNamespacesIsNil.Spec.Install = addonsv1alpha1.AddonInstallSpec{
			Type: addonsv1alpha1.OLMAllNamespaces,
		}

		addonAllNamespacesIsEmpty := testutil.NewTestAddonWithoutNamespace()
		addonAllNamespacesIsEmpty.Spec.Install = addonsv1alpha1.AddonInstallSpec{
			Type:             addonsv1alpha1.OLMAllNamespaces,
			OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{},
		}

		tests := []struct {
			name  string
			addon *addonsv1alpha1.Addon
		}{
			{
				name:  "ownNamespace is nil",
				addon: addonOwnNamespaceIsNil,
			},
			{
				name:  "ownNamespace.namespace is empty",
				addon: addonOwnNamespaceIsEmpty,
			},
			{
				name:  "allNamespaces is nil",
				addon: addonAllNamespacesIsNil,
			},
			{
				name:  "allNamespaces.namespace is empty",
				addon: addonAllNamespacesIsEmpty,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				log := testutil.NewLogger(t)
				c := testutil.NewClient()
				r := &olmReconciler{
					client: c,
					scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
				}

				// Test
				ctx := controllers.ContextWithLogger(context.Background(), log)
				requeueResult, err := r.ensureOperatorGroup(ctx, test.addon)
				require.NoError(t, err)
				assert.Equal(t, resultStop, requeueResult)

				availableCond := meta.FindStatusCondition(test.addon.Status.Conditions, addonsv1alpha1.Available)
				if assert.NotNil(t, availableCond) {
					assert.Equal(t, metav1.ConditionFalse, availableCond.Status)
					assert.Equal(t, addonsv1alpha1.AddonReasonConfigError, availableCond.Reason)
				}
			})
		}
	})

	t.Run("unsupported install type", func(t *testing.T) {
		addonUnsupportedInstallType := testutil.NewTestAddonWithoutNamespace()
		addonUnsupportedInstallType.Spec.Install = addonsv1alpha1.AddonInstallSpec{
			Type: addonsv1alpha1.AddonInstallType("something something"),
		}

		log := testutil.NewLogger(t)
		c := testutil.NewClient()
		r := &olmReconciler{
			client: c,
			scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
		}

		// Test
		ctx := controllers.ContextWithLogger(context.Background(), log)
		requeueResult, err := r.ensureOperatorGroup(ctx, addonUnsupportedInstallType.DeepCopy())
		require.NoError(t, err)
		assert.Equal(t, resultStop, requeueResult)

		// indirect sanity check
		// nothing was called on the client and the method signals to stop
	})
}

func TestReconcileOperatorGroup_Adoption(t *testing.T) {
	for name, tc := range map[string]struct {
		AlreadyOwnedByAddon bool
	}{
		"Operator group not owned by addon": {
			AlreadyOwnedByAddon: false,
		},
		"Operator group already owned by addon": {
			AlreadyOwnedByAddon: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			operatorGroup := testutil.NewTestOperatorGroup()
			c := testutil.NewClient()

			c.On("Get",
				testutil.IsContext,
				testutil.IsObjectKey,
				testutil.IsOperatorsV1OperatorGroupPtr,
				mock.Anything,
			).Run(func(args mock.Arguments) {
				var og *operatorsv1.OperatorGroup

				if tc.AlreadyOwnedByAddon {
					og = testutil.NewTestOperatorGroup()
					// Unrelated spec change to force reconciliation
					og.Spec.StaticProvidedAPIs = true
				} else {
					og = testutil.NewTestOperatorGroupWithoutOwner()
				}

				og.DeepCopyInto(args.Get(2).(*operatorsv1.OperatorGroup))
			}).Return(nil)

			c.On("Update",
				testutil.IsContext,
				testutil.IsOperatorsV1OperatorGroupPtr,
				mock.Anything,
			).Return(nil)

			rec := &olmReconciler{
				client: c,
				scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
			}

			ctx := context.Background()
			err := rec.reconcileOperatorGroup(ctx, operatorGroup.DeepCopy())

			assert.NoError(t, err)
			c.AssertExpectations(t)
		})
	}
}

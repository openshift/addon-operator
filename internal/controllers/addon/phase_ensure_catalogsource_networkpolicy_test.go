package addon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"

	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestReconcileCatalogSourceNetworkPolicy_InvalidAddon(t *testing.T) {
	for name, tc := range map[string]struct {
		Addon *addonsv1alpha1.Addon
	}{
		"addon no install spec": {
			Addon: &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-addon",
				},
				Spec: addonsv1alpha1.AddonSpec{},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &olmReconciler{
				scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
			}

			res, err := rec.ensureCatalogSourcesNetworkPolicy(context.Background(), tc.Addon)
			require.NoError(t, err)

			assert.Equal(t, resultStop, res)
		})
	}
}

func TestReconcileCatalogSourceNetworkPolicy_NotPresent(t *testing.T) {
	for name, tc := range map[string]struct {
		Addon    *addonsv1alpha1.Addon
		Expected *networkingv1.NetworkPolicy
	}{
		"addon with no additional catalog sources": {
			Addon:    newNetworkPolicyTestAddon(),
			Expected: newTestNetworkPolicy(),
		},
		"addon with additional catalog sources": {
			Addon: newNetworkPolicyTestAddon(
				npTestAddonAdditionalCatalogSourceNames(
					"foo-catalog-source",
					"bar-catalog-source",
				),
			),
			Expected: newTestNetworkPolicy(
				testNetworkPolicyMatchExpressions(
					metav1.LabelSelectorRequirement{
						Key:      "olm.catalogSource",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"addon-foo-addon-catalog",
							"foo-catalog-source",
							"bar-catalog-source",
						},
					},
				),
			),
		},
		"addon with name 'bar-addon'": {
			Addon: newNetworkPolicyTestAddon(
				npTestAddonName("bar-addon"),
			),
			Expected: newTestNetworkPolicy(
				testNetworkPolicyName("addon-bar-addon-catalogs"),
				testNetworkPolicyMatchExpressions(
					metav1.LabelSelectorRequirement{
						Key:      "olm.catalogSource",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"addon-bar-addon-catalog"},
					},
				),
			),
		},
		"addon with namespace 'bar-addon'": {
			Addon: newNetworkPolicyTestAddon(
				npTestAddonNamespace("bar-addon"),
			),
			Expected: newTestNetworkPolicy(
				testNetworkPolicyNamespace("bar-addon"),
			),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var created *networkingv1.NetworkPolicy

			client := testutil.NewClient()
			client.
				On("Get",
					testutil.IsContext,
					mock.IsType(types.NamespacedName{}),
					testutil.IsNetworkingV1NetworkPolicyPtr,
					mock.Anything).
				Return(testutil.NewTestErrNotFound())
			client.
				On("Create",
					testutil.IsContext,
					testutil.IsNetworkingV1NetworkPolicyPtr,
					mock.Anything).
				Run(func(args mock.Arguments) {
					created = args.Get(1).(*networkingv1.NetworkPolicy)
				}).
				Return(nil)

			rec := &olmReconciler{
				client: client,
				scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
			}

			_, err := rec.ensureCatalogSourcesNetworkPolicy(context.Background(), tc.Addon)
			require.NoError(t, err)

			assert.Equal(t, tc.Expected.Name, created.Name)
			assert.Equal(t, tc.Expected.Namespace, created.Namespace)
			assert.ElementsMatch(t, tc.Expected.Spec.PodSelector.MatchExpressions, created.Spec.PodSelector.MatchExpressions)
			assert.ElementsMatch(t, tc.Expected.Spec.Ingress, created.Spec.Ingress)
			assert.ElementsMatch(t, tc.Expected.Spec.PolicyTypes, created.Spec.PolicyTypes)

			client.AssertExpectations(t)
		})
	}
}

func TestReconcileCatalogSourceNetworkPolicy_Present(t *testing.T) {
	for name, tc := range map[string]struct {
		Addon                 *addonsv1alpha1.Addon
		ActualNetworkPolicy   *networkingv1.NetworkPolicy
		ExpectedNetworkPolicy *networkingv1.NetworkPolicy
	}{
		"existing NetworkPolicy with no MatchExpressions": {
			Addon:                 newNetworkPolicyTestAddon(),
			ActualNetworkPolicy:   newTestNetworkPolicy(),
			ExpectedNetworkPolicy: newTestNetworkPolicy(),
		},
		"existing NetworkPolicy with missing CatalogSource Names": {
			Addon: newNetworkPolicyTestAddon(
				npTestAddonAdditionalCatalogSourceNames(
					"foo-catalog-source",
					"bar-catalog-source",
				),
			),
			ActualNetworkPolicy: newTestNetworkPolicy(),
			ExpectedNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyMatchExpressions(
					metav1.LabelSelectorRequirement{
						Key:      "olm.catalogSource",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"addon-foo-addon-catalog",
							"foo-catalog-source",
							"bar-catalog-source",
						},
					},
				),
			),
		},
		"existing NetworkPolicy with bad ingress rule": {
			Addon: newNetworkPolicyTestAddon(),
			ActualNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyIngress(
					networkingv1.NetworkPolicyIngressRule{
						Ports: []networkingv1.NetworkPolicyPort{
							{
								Protocol: corev1ProtocolPtr(corev1.ProtocolTCP),
								Port:     intOrStringPtr(intstr.FromInt(60061)),
							},
						},
					},
				),
			),
			ExpectedNetworkPolicy: newTestNetworkPolicy(),
		},
		"existing NetworkPolicy with bad policy type": {
			Addon: newNetworkPolicyTestAddon(),
			ActualNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyTypes(
					networkingv1.PolicyTypeEgress,
				),
			),
			ExpectedNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyMatchExpressions(
					metav1.LabelSelectorRequirement{
						Key:      "olm.catalogSource",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"addon-foo-addon-catalog"},
					},
				),
			),
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, controllerutil.SetControllerReference(
				tc.Addon,
				tc.ActualNetworkPolicy,
				testutil.NewTestSchemeWithAddonsv1alpha1()),
			)

			var updated *networkingv1.NetworkPolicy

			client := testutil.NewClient()
			client.
				On("Get",
					testutil.IsContext,
					mock.IsType(types.NamespacedName{}),
					testutil.IsNetworkingV1NetworkPolicyPtr,
					mock.Anything).
				Run(func(args mock.Arguments) {
					tc.ActualNetworkPolicy.DeepCopyInto(args.Get(2).(*networkingv1.NetworkPolicy))
				}).
				Return(nil)
			client.
				On("Update",
					testutil.IsContext,
					testutil.IsNetworkingV1NetworkPolicyPtr,
					mock.Anything).
				Run(func(args mock.Arguments) {
					updated = args.Get(1).(*networkingv1.NetworkPolicy)
				}).
				Return(nil)

			rec := &olmReconciler{
				client: client,
				scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
			}

			_, err := rec.ensureCatalogSourcesNetworkPolicy(context.Background(), tc.Addon)
			require.NoError(t, err)

			assert.Equal(t, tc.ExpectedNetworkPolicy.Spec, updated.Spec)

			client.AssertExpectations(t)
		})
	}
}

func TestEnsureCatalogSourceNetworkPolicy_Adoption(t *testing.T) {
	for name, tc := range map[string]struct {
		ActualNetworkPolicy *networkingv1.NetworkPolicy
		AlreadyOwned        bool
		Strategy            addonsv1alpha1.ResourceAdoptionStrategyType
		Expected            error
	}{
		"existing NetworkPolicy with no owner/no strategy": {
			ActualNetworkPolicy: newTestNetworkPolicy(),
			AlreadyOwned:        false,
			Strategy:            addonsv1alpha1.ResourceAdoptionStrategyType(""),
			Expected:            controllers.ErrNotOwnedByUs,
		},
		"existing NetworkPolicy with no owner/Prevent": {
			ActualNetworkPolicy: newTestNetworkPolicy(),
			AlreadyOwned:        false,
			Strategy:            addonsv1alpha1.ResourceAdoptionPrevent,
			Expected:            controllers.ErrNotOwnedByUs,
		},
		"existing NetworkPolicy with no owner/AdoptAll": {
			ActualNetworkPolicy: newTestNetworkPolicy(),
			AlreadyOwned:        false,
			Strategy:            addonsv1alpha1.ResourceAdoptionAdoptAll,
			Expected:            nil,
		},
		"existing NetworkPolicy addon owned/no strategy": {
			ActualNetworkPolicy: newTestNetworkPolicy(),
			AlreadyOwned:        true,
			Strategy:            addonsv1alpha1.ResourceAdoptionStrategyType(""),
			Expected:            nil,
		},
		"existing NetworkPolicy addon owned/Prevent": {
			ActualNetworkPolicy: newTestNetworkPolicy(),
			AlreadyOwned:        true,
			Strategy:            addonsv1alpha1.ResourceAdoptionPrevent,
			Expected:            nil,
		},
		"existing NetworkPolicy addon owned/AdoptAll": {
			ActualNetworkPolicy: newTestNetworkPolicy(),
			AlreadyOwned:        true,
			Strategy:            addonsv1alpha1.ResourceAdoptionAdoptAll,
			Expected:            nil,
		},
		"existing NetworkPolicy with altered spec/no strategy": {
			ActualNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyTypes(networkingv1.PolicyTypeEgress),
			),
			AlreadyOwned: false,
			Strategy:     addonsv1alpha1.ResourceAdoptionStrategyType(""),
			Expected:     controllers.ErrNotOwnedByUs,
		},
		"existing NetworkPolicy with altered spec/Prevent": {
			ActualNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyTypes(networkingv1.PolicyTypeEgress),
			),
			AlreadyOwned: false,
			Strategy:     addonsv1alpha1.ResourceAdoptionPrevent,
			Expected:     controllers.ErrNotOwnedByUs,
		},
		"existing NetworkPolicy with altered spec/AdoptAll": {
			ActualNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyTypes(networkingv1.PolicyTypeEgress),
			),
			AlreadyOwned: false,
			Strategy:     addonsv1alpha1.ResourceAdoptionAdoptAll,
			Expected:     nil,
		},
		"existing NetworkPolicy with altered spec and addon owned/no strategy": {
			ActualNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyTypes(networkingv1.PolicyTypeEgress),
			),
			AlreadyOwned: true,
			Strategy:     addonsv1alpha1.ResourceAdoptionStrategyType(""),
			Expected:     nil,
		},
		"existing NetworkPolicy with altered spec and addon owned/Prevent": {
			ActualNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyTypes(networkingv1.PolicyTypeEgress),
			),
			AlreadyOwned: true,
			Strategy:     addonsv1alpha1.ResourceAdoptionPrevent,
			Expected:     nil,
		},
		"existing NetworkPolicy with altered spec and addon owned/AdoptAll": {
			ActualNetworkPolicy: newTestNetworkPolicy(
				testNetworkPolicyTypes(networkingv1.PolicyTypeEgress),
			),
			AlreadyOwned: true,
			Strategy:     addonsv1alpha1.ResourceAdoptionAdoptAll,
			Expected:     nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			addon := newNetworkPolicyTestAddon()
			addon.Spec.ResourceAdoptionStrategy = tc.Strategy

			if tc.AlreadyOwned {
				require.NoError(t, controllerutil.SetControllerReference(
					addon,
					tc.ActualNetworkPolicy,
					testutil.NewTestSchemeWithAddonsv1alpha1()),
				)
			}

			client := testutil.NewClient()
			client.
				On("Get",
					testutil.IsContext,
					mock.IsType(types.NamespacedName{}),
					testutil.IsNetworkingV1NetworkPolicyPtr,
					mock.Anything).
				Run(func(args mock.Arguments) {
					tc.ActualNetworkPolicy.DeepCopyInto(args.Get(2).(*networkingv1.NetworkPolicy))
				}).
				Return(nil)
			client.
				On("Update",
					testutil.IsContext,
					testutil.IsNetworkingV1NetworkPolicyPtr,
					mock.Anything).
				Return(nil).
				Maybe()

			rec := &olmReconciler{
				client: client,
				scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
			}

			_, err := rec.ensureCatalogSourcesNetworkPolicy(context.Background(), addon)
			require.ErrorIs(t, err, tc.Expected)

			client.AssertExpectations(t)
		})
	}
}

func newNetworkPolicyTestAddon(opts ...npTestAddonOption) *addonsv1alpha1.Addon {
	cfg := npTestAddonConfig{
		Name:      "foo-addon",
		Namespace: "foo-addon",
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	res := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfg.Name,
		},
		Spec: addonsv1alpha1.AddonSpec{
			Install: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						Namespace:          cfg.Namespace,
						CatalogSourceImage: "quay.io/osd-addons/foo-catalog-source",
					},
				},
			},
		},
	}

	if len(cfg.AdditionalCatalogSourceNames) > 0 {
		var acs []addonsv1alpha1.AdditionalCatalogSource

		for _, name := range cfg.AdditionalCatalogSourceNames {
			acs = append(acs, addonsv1alpha1.AdditionalCatalogSource{
				Name: name,
			})
		}

		res.Spec.Install.OLMOwnNamespace.AdditionalCatalogSources = acs
	}

	return res
}

type npTestAddonConfig struct {
	Name                         string
	Namespace                    string
	AdditionalCatalogSourceNames []string
}

type npTestAddonOption func(*npTestAddonConfig)

func npTestAddonName(name string) npTestAddonOption {
	return func(c *npTestAddonConfig) {
		c.Name = name
	}
}

func npTestAddonNamespace(ns string) npTestAddonOption {
	return func(c *npTestAddonConfig) {
		c.Namespace = ns
	}
}

func npTestAddonAdditionalCatalogSourceNames(names ...string) npTestAddonOption {
	return func(c *npTestAddonConfig) {
		c.AdditionalCatalogSourceNames = names
	}
}

func newTestNetworkPolicy(opts ...testNetworkPolicyOption) *networkingv1.NetworkPolicy {
	cfg := testNetworkPolicyConfig{
		Name:      "addon-foo-addon-catalogs",
		Namespace: "foo-addon",
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "olm.catalogSource",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"addon-foo-addon-catalog"},
			},
		},
		Ingress:     expectedIngress(),
		PolicyTypes: expectedPolicyTypes(),
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchExpressions: cfg.MatchExpressions,
			},
			Ingress:     cfg.Ingress,
			PolicyTypes: cfg.PolicyTypes,
		},
	}
}

type testNetworkPolicyConfig struct {
	Name             string
	Namespace        string
	MatchExpressions []metav1.LabelSelectorRequirement
	Ingress          []networkingv1.NetworkPolicyIngressRule
	PolicyTypes      []networkingv1.PolicyType
}

type testNetworkPolicyOption func(*testNetworkPolicyConfig)

func testNetworkPolicyName(name string) testNetworkPolicyOption {
	return func(c *testNetworkPolicyConfig) {
		c.Name = name
	}
}

func testNetworkPolicyNamespace(ns string) testNetworkPolicyOption {
	return func(c *testNetworkPolicyConfig) {
		c.Namespace = ns
	}
}

func testNetworkPolicyMatchExpressions(exprs ...metav1.LabelSelectorRequirement) testNetworkPolicyOption {
	return func(c *testNetworkPolicyConfig) {
		c.MatchExpressions = exprs
	}
}

func testNetworkPolicyIngress(ingress ...networkingv1.NetworkPolicyIngressRule) testNetworkPolicyOption {
	return func(c *testNetworkPolicyConfig) {
		c.Ingress = ingress
	}
}

func testNetworkPolicyTypes(pts ...networkingv1.PolicyType) testNetworkPolicyOption {
	return func(c *testNetworkPolicyConfig) {
		c.PolicyTypes = pts
	}
}

func expectedIngress() []networkingv1.NetworkPolicyIngressRule {
	return []networkingv1.NetworkPolicyIngressRule{
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: corev1ProtocolPtr(corev1.ProtocolTCP),
					Port:     intOrStringPtr(intstr.FromInt(50051)),
				},
			},
		},
	}
}

func expectedPolicyTypes() []networkingv1.PolicyType {
	return []networkingv1.PolicyType{
		networkingv1.PolicyTypeIngress,
	}
}

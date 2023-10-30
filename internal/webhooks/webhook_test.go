package webhooks

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

func Test_validateInstallSpec(t *testing.T) {
	testCases := []struct {
		name             string
		addonInstallSpec addonsv1alpha1.AddonInstallSpec
		addonName        string
		expectedErr      error
	}{
		{
			name:             "missing install type",
			addonInstallSpec: addonsv1alpha1.AddonInstallSpec{},
			expectedErr:      errSpecInstallTypeInvalid,
		},
		{
			name: "invalid install type",
			addonInstallSpec: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.AddonInstallType("This is not valid"),
			},
			expectedErr: errSpecInstallTypeInvalid,
		},
		{
			name: "spec.install.ownNamespace required",
			addonInstallSpec: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
			},
			expectedErr: errSpecInstallOwnNamespaceRequired,
		},
		{
			name: "spec.install.allNamespaces required",
			addonInstallSpec: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMAllNamespaces,
			},
			expectedErr: errSpecInstallAllNamespacesRequired,
		},
		{
			name: "spec.install.allNamespaces and *.ownNamespace mutually exclusive",
			addonInstallSpec: addonsv1alpha1.AddonInstallSpec{
				Type:             addonsv1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{},
				OLMOwnNamespace:  &addonsv1alpha1.AddonInstallOLMOwnNamespace{},
			},
			expectedErr: errSpecInstallConfigMutuallyExclusive,
		},
		{
			name: "main catalog and additional catalog source name collision",
			addonInstallSpec: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
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
			addonName:   "test-2",
			expectedErr: errAdditionalCatalogSourceNameCollision,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateInstallSpec(tc.addonInstallSpec, tc.addonName)
			assert.EqualValues(t, tc.expectedErr, err)
		})
	}
}

func TestValidateAddonInstallImmutability(t *testing.T) {
	var (
		addonName     = "test-addon"
		catalogSource = "quay.io/osd-addons/reference-addon-index"
	)

	baseAddon := testutil.NewAddonWithInstallSpec(addonsv1alpha1.AddonInstallSpec{
		Type: addonsv1alpha1.OLMAllNamespaces,
		OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
			AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
				Namespace:          "reference-addon",
				PackageName:        addonName,
				Channel:            "alpha",
				CatalogSourceImage: catalogSource,
			},
		},
	}, addonName)

	baseAddon_withEnv := testutil.NewAddonWithInstallSpec(addonsv1alpha1.AddonInstallSpec{
		Type: addonsv1alpha1.OLMAllNamespaces,
		OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
			AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
				Namespace:          "reference-addon",
				PackageName:        addonName,
				Channel:            "alpha",
				CatalogSourceImage: catalogSource,
				Config: &addonsv1alpha1.SubscriptionConfig{
					EnvironmentVariables: []addonsv1alpha1.EnvObject{
						{
							Name:  "key-1",
							Value: "value-1",
						},
					},
				},
			},
		},
	}, addonName)

	testCases := []struct {
		baseAddon    *addonsv1alpha1.Addon
		updatedAddon *addonsv1alpha1.Addon
		expectedErr  error
	}{
		{
			baseAddon:    baseAddon,
			updatedAddon: baseAddon,
			expectedErr:  nil,
		},
		{
			baseAddon: baseAddon,
			updatedAddon: testutil.NewAddonWithInstallSpec(addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						Namespace:          "reference-addon",
						PackageName:        addonName,
						Channel:            "beta", // changed
						CatalogSourceImage: catalogSource,
					},
				},
			}, addonName),
			expectedErr: nil,
		},
		{
			baseAddon: baseAddon,
			updatedAddon: testutil.NewAddonWithInstallSpec(addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						Namespace:          "reference-addon",
						PackageName:        addonName,
						Channel:            "alpha",
						CatalogSourceImage: "some-other-catalogsource", // changed
					},
				},
			}, addonName),
			expectedErr: nil,
		},
		{
			baseAddon: baseAddon,
			updatedAddon: testutil.NewAddonWithInstallSpec(addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
			}, addonName),
			expectedErr: errInstallTypeImmutable,
		},
		{
			baseAddon: baseAddon,
			updatedAddon: testutil.NewAddonWithInstallSpec(addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						Namespace:          "reference-addon",
						PackageName:        addonName,
						Channel:            "alpha",
						CatalogSourceImage: catalogSource,
						// changed (added)
						Config: &addonsv1alpha1.SubscriptionConfig{
							EnvironmentVariables: []addonsv1alpha1.EnvObject{
								{
									Name:  "key1",
									Value: "value1",
								},
							},
						},
					},
				},
			}, addonName),
			expectedErr: nil,
		},
		{
			baseAddon: baseAddon_withEnv,
			updatedAddon: testutil.NewAddonWithInstallSpec(addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						Namespace:          "reference-addon",
						PackageName:        addonName,
						Channel:            "alpha",
						CatalogSourceImage: catalogSource,
						// changed
						Config: &addonsv1alpha1.SubscriptionConfig{
							EnvironmentVariables: []addonsv1alpha1.EnvObject{
								{
									Name:  "key-2",
									Value: "value-2",
								},
							},
						},
					},
				},
			}, addonName),
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run("addon install immutability test", func(t *testing.T) {
			err := validateAddonImmutability(tc.updatedAddon, tc.baseAddon)
			assert.EqualValues(t, tc.expectedErr, err)
		})
	}
}

func TestValidateSecretPropagation(t *testing.T) {
	testCases := []struct {
		addon       *addonsv1alpha1.Addon
		expectedErr error
	}{
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			expectedErr: nil,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			expectedErr: nil,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &addonsv1alpha1.AddonSecretPropagation{
						Secrets: []addonsv1alpha1.AddonSecretPropagationReference{
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test",
								},
							},
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "foo",
								},
							},
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &addonsv1alpha1.AddonSecretPropagation{
						Secrets: []addonsv1alpha1.AddonSecretPropagationReference{
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "foo",
								},
							},
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test",
								},
							},
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &addonsv1alpha1.AddonSecretPropagation{
						Secrets: []addonsv1alpha1.AddonSecretPropagationReference{
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test-1",
								},
							},
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test-2",
								},
							},
						},
					},
				},
			},
			expectedErr: fmt.Errorf("pullSecretName %q not found as destination in secretPropagation", "test"),
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &addonsv1alpha1.AddonSecretPropagation{
						Secrets: []addonsv1alpha1.AddonSecretPropagationReference{
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test-1",
								},
							},
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test-2",
								},
							},
						},
					},
				},
			},
			expectedErr: fmt.Errorf("pullSecretName %q not found as destination in secretPropagation", "test"),
		},
	}

	for _, tc := range testCases {
		t.Run("validate secret propagation test", func(t *testing.T) {
			addon := tc.addon.DeepCopy()
			err := validateSecretPropagation(addon)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

func TestValidateAddon(t *testing.T) {
	testCases := []struct {
		addon       *addonsv1alpha1.Addon
		expectedErr error
	}{
		{
			addon: &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{},
				},
			},
			expectedErr: errSpecInstallTypeInvalid,
		},
		{
			addon: &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type:            addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: nil,
					},
				},
			},
			expectedErr: errSpecInstallOwnNamespaceRequired,
		},
		{
			addon: &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name: "test",
									},
								},
							},
						},
					},
				},
			},
			expectedErr: errAdditionalCatalogSourceNameCollision,
		},
		{
			addon: &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type:             addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: nil,
					},
				},
			},
			expectedErr: errSpecInstallAllNamespacesRequired,
		},
		{
			addon: &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								AdditionalCatalogSources: []addonsv1alpha1.AdditionalCatalogSource{
									{
										Name: "test",
									},
								},
							},
						},
					},
				},
			},
			expectedErr: errAdditionalCatalogSourceNameCollision,
		},
		{
			addon: &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type:             addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{},
						OLMOwnNamespace:  &addonsv1alpha1.AddonInstallOLMOwnNamespace{},
					},
				},
			},
			expectedErr: errSpecInstallConfigMutuallyExclusive,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &addonsv1alpha1.AddonSecretPropagation{
						Secrets: []addonsv1alpha1.AddonSecretPropagationReference{
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test-1",
								},
							},
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test-2",
								},
							},
						},
					},
				},
			},
			expectedErr: fmt.Errorf("pullSecretName %q not found as destination in secretPropagation", "test"),
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &addonsv1alpha1.AddonSecretPropagation{
						Secrets: []addonsv1alpha1.AddonSecretPropagationReference{
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "foo",
								},
							},
							{
								DestinationSecret: corev1.LocalObjectReference{
									Name: "test",
								},
							},
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					Install: addonsv1alpha1.AddonInstallSpec{
						Type: addonsv1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &addonsv1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run("validate addon tests", func(t *testing.T) {
			addon := tc.addon.DeepCopy()
			err := validateAddon(addon)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

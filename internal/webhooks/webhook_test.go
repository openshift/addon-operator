package webhooks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
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
			expectedErr: errInstallImmutable,
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

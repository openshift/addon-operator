package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"

	av1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestDefaultAddonValidatorInterfaces(t *testing.T) {
	t.Parallel()

	require.Implements(t, new(admission.CustomValidator), new(DefaultAddonValidator))
}

func TestDefaultAddonValidator_ValidateCreate(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		Obj         runtime.Object
		ExpectedErr error
	}{
		"missing install type": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{},
				},
			},
			ExpectedErr: errSpecInstallTypeInvalid,
		},
		"invalid install type": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.AddonInstallType("This is not valid"),
					},
				},
			},
			ExpectedErr: errSpecInstallTypeInvalid,
		},
		"spec.install.ownNamespace required": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
					},
				},
			},
			ExpectedErr: errSpecInstallOwnNamespaceRequired,
		},
		"spec.install.allNamespaces required": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
					},
				},
			},
			ExpectedErr: errSpecInstallAllNamespacesRequired,
		},
		"spec.install.allNamespaces and *.ownNamespace mutually exclusive": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type:             av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{},
						OLMOwnNamespace:  &av1alpha1.AddonInstallOLMOwnNamespace{},
					},
				},
			},
			ExpectedErr: errSpecInstallConfigMutuallyExclusive,
		},
		"main catalog and additional catalog source name collision": {
			Obj: &av1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-2",
				},
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								AdditionalCatalogSources: []av1alpha1.AdditionalCatalogSource{
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
			ExpectedErr: errAdditionalCatalogSourceNameCollision,
		},
		"empty PullSecretName/OLMOwnNamespace": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			ExpectedErr: nil,
		},
		"empty PullSecretName/OLMAllNamespaces": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMOwnNamespace/nil SecretPropagation": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMAllNamespaces/nil SecretPropagation": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMOwnNamespaces/with SecretPropagation": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMAllNamespaces/with SecretPropagation": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMOwnNamespace/with invalid SecretPropagation": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			ExpectedErr: errInvalidPullSecretConfiguration,
		},
		"non-empty pullSecretName/OLMAllNamespaces/with invalid SecretPropagation": {
			Obj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			ExpectedErr: errInvalidPullSecretConfiguration,
		},
	} {
		tc := tc

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			validator := NewDefaultAddonValidator()

			_, err := validator.ValidateCreate(context.Background(), tc.Obj)
			assert.ErrorIs(t, err, tc.ExpectedErr)
		})
	}
}

func TestDefaultAddonValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()

	const (
		addonName     = "test-addon"
		catalogSource = "quay.io/osd-addons/reference-addon-index"
	)

	baseAddon := testutil.NewAddonWithInstallSpec(av1alpha1.AddonInstallSpec{
		Type: av1alpha1.OLMAllNamespaces,
		OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
			AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
				Namespace:          "reference-addon",
				PackageName:        addonName,
				Channel:            "alpha",
				CatalogSourceImage: catalogSource,
			},
		},
	}, addonName)

	baseAddonWithEnv := testutil.NewAddonWithInstallSpec(av1alpha1.AddonInstallSpec{
		Type: av1alpha1.OLMAllNamespaces,
		OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
			AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
				Namespace:          "reference-addon",
				PackageName:        addonName,
				Channel:            "alpha",
				CatalogSourceImage: catalogSource,
				Config: &av1alpha1.SubscriptionConfig{
					EnvironmentVariables: []av1alpha1.EnvObject{
						{
							Name:  "key-1",
							Value: "value-1",
						},
					},
				},
			},
		},
	}, addonName)

	for name, tc := range map[string]struct {
		OldObj      runtime.Object
		NewObj      runtime.Object
		ExpectedErr error
	}{
		"missing install type": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{},
				},
			},
			OldObj:      baseAddon,
			ExpectedErr: errSpecInstallTypeInvalid,
		},
		"invalid install type": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.AddonInstallType("This is not valid"),
					},
				},
			},
			OldObj:      baseAddon,
			ExpectedErr: errSpecInstallTypeInvalid,
		},
		"spec.install.ownNamespace required": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
					},
				},
			},
			OldObj:      baseAddon,
			ExpectedErr: errSpecInstallOwnNamespaceRequired,
		},
		"spec.install.allNamespaces required": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
					},
				},
			},
			OldObj:      baseAddon,
			ExpectedErr: errSpecInstallAllNamespacesRequired,
		},
		"spec.install.allNamespaces and *.ownNamespace mutually exclusive": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type:             av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{},
						OLMOwnNamespace:  &av1alpha1.AddonInstallOLMOwnNamespace{},
					},
				},
			},
			OldObj:      baseAddon,
			ExpectedErr: errSpecInstallConfigMutuallyExclusive,
		},
		"main catalog and additional catalog source name collision": {
			NewObj: &av1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-2",
				},
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								AdditionalCatalogSources: []av1alpha1.AdditionalCatalogSource{
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
			OldObj:      baseAddon,
			ExpectedErr: errAdditionalCatalogSourceNameCollision,
		},
		"empty PullSecretName/OLMOwnNamespace": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			OldObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			ExpectedErr: nil,
		},
		"empty PullSecretName/OLMAllNamespaces": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			OldObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "",
							},
						},
					},
				},
			},
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMOwnNamespace/nil SecretPropagation": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			OldObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMAllNamespaces/nil SecretPropagation": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			OldObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: nil,
				},
			},
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMOwnNamespaces/with SecretPropagation": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			OldObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMAllNamespaces/with SecretPropagation": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			OldObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			ExpectedErr: nil,
		},
		"non-empty pullSecretName/OLMOwnNamespace/with invalid SecretPropagation": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMOwnNamespace,
						OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			OldObj:      baseAddon,
			ExpectedErr: errInvalidPullSecretConfiguration,
		},
		"non-empty pullSecretName/OLMAllNamespaces/with invalid SecretPropagation": {
			NewObj: &av1alpha1.Addon{
				Spec: av1alpha1.AddonSpec{
					Install: av1alpha1.AddonInstallSpec{
						Type: av1alpha1.OLMAllNamespaces,
						OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
							AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
								PullSecretName: "test",
							},
						},
					},
					SecretPropagation: &av1alpha1.AddonSecretPropagation{
						Secrets: []av1alpha1.AddonSecretPropagationReference{
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
			OldObj:      baseAddon,
			ExpectedErr: errInvalidPullSecretConfiguration,
		},
		"no change": {
			OldObj:      baseAddon,
			NewObj:      baseAddon,
			ExpectedErr: nil,
		},
		"updated channel": {
			OldObj: baseAddon,
			NewObj: testutil.NewAddonWithInstallSpec(av1alpha1.AddonInstallSpec{
				Type: av1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
					AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
						Namespace:          "reference-addon",
						PackageName:        addonName,
						Channel:            "beta", // changed
						CatalogSourceImage: catalogSource,
					},
				},
			}, addonName),
			ExpectedErr: nil,
		},
		"updated catalogSourceImage": {
			OldObj: baseAddon,
			NewObj: testutil.NewAddonWithInstallSpec(av1alpha1.AddonInstallSpec{
				Type: av1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
					AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
						Namespace:          "reference-addon",
						PackageName:        addonName,
						Channel:            "alpha",
						CatalogSourceImage: "some-other-catalogsource", // changed
					},
				},
			}, addonName),
			ExpectedErr: nil,
		},
		"updated install type": {
			OldObj: baseAddon,
			NewObj: testutil.NewAddonWithInstallSpec(av1alpha1.AddonInstallSpec{
				Type:            av1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &av1alpha1.AddonInstallOLMOwnNamespace{},
			}, addonName),
			ExpectedErr: errInstallTypeImmutable,
		},
		"added subscription config": {
			OldObj: baseAddon,
			NewObj: testutil.NewAddonWithInstallSpec(av1alpha1.AddonInstallSpec{
				Type: av1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
					AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
						Namespace:          "reference-addon",
						PackageName:        addonName,
						Channel:            "alpha",
						CatalogSourceImage: catalogSource,
						// changed (added)
						Config: &av1alpha1.SubscriptionConfig{
							EnvironmentVariables: []av1alpha1.EnvObject{
								{
									Name:  "key1",
									Value: "value1",
								},
							},
						}}},
			}, addonName),
			ExpectedErr: nil,
		},
		"updated subscription config": {
			OldObj: baseAddonWithEnv,
			NewObj: testutil.NewAddonWithInstallSpec(av1alpha1.AddonInstallSpec{
				Type: av1alpha1.OLMAllNamespaces,
				OLMAllNamespaces: &av1alpha1.AddonInstallOLMAllNamespaces{
					AddonInstallOLMCommon: av1alpha1.AddonInstallOLMCommon{
						Namespace:          "reference-addon",
						PackageName:        addonName,
						Channel:            "alpha",
						CatalogSourceImage: catalogSource,
						// changed
						Config: &av1alpha1.SubscriptionConfig{
							EnvironmentVariables: []av1alpha1.EnvObject{
								{
									Name:  "key-2",
									Value: "value-2",
								},
							},
						},
					},
				},
			}, addonName),
			ExpectedErr: nil,
		},
	} {
		tc := tc

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			validator := NewDefaultAddonValidator()

			_, err := validator.ValidateUpdate(context.Background(), tc.OldObj, tc.NewObj)
			assert.ErrorIs(t, err, tc.ExpectedErr)
		})
	}
}

func TestDefaultAddonValidator_ValidateDelete(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		Object      runtime.Object
		ExpectedErr error
	}{
		"any addon": {
			Object:      &av1alpha1.Addon{},
			ExpectedErr: nil,
		},
	} {
		tc := tc

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			validator := NewDefaultAddonValidator()

			_, err := validator.ValidateDelete(context.Background(), tc.Object)
			assert.ErrorIs(t, err, tc.ExpectedErr)
		})
	}
}

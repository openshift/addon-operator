package addon

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestReconcileSecret_CreateWithClientError(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lorem-ipsum",
			Namespace: "default",
		},
	}

	timeoutErr := k8sApiErrors.NewTimeoutError("for testing", 1)

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext,
		testutil.IsObjectKey,
		mock.IsType(&corev1.Secret{})).
		Return(timeoutErr)

	ctx := context.Background()
	err := reconcileSecret(ctx, c, secret)
	require.Error(t, err)
	require.ErrorIs(t, err, timeoutErr)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", testutil.IsContext, client.ObjectKey{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}, mock.IsType(&corev1.Secret{}))
}

func Test_getReferencedPullSecret_uncachedFallback(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{},
		Spec: addonsv1alpha1.AddonSpec{
			Namespaces: []addonsv1alpha1.AddonNamespace{
				{Name: "test"},
			},
			Install: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						PullSecretName:     "lorem-ipsum",
						CatalogSourceImage: "xxx",
						Namespace:          "test",
					},
				},
			},
		},
	}
	addonPullSecretKey := client.ObjectKey{Name: "lorem-ipsum", Namespace: "xxx-addon-operator"}

	c := testutil.NewClient()
	uncachedC := testutil.NewClient()
	c.
		On("Get", // get referenced Secret
			testutil.IsContext,
			addonPullSecretKey,
			mock.IsType(&corev1.Secret{}),
		).
		Return(testutil.NewTestErrNotFound())
	uncachedC.
		On("Get", // get referenced Secret via uncached read
			testutil.IsContext,
			addonPullSecretKey,
			mock.IsType(&corev1.Secret{}),
		).
		Return(nil)

	r := &AddonReconciler{
		Client:                 c,
		UncachedClient:         uncachedC,
		Log:                    testutil.NewLogger(t),
		Scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		AddonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	secret, result, err := getReferencedSecret(ctx, r.Log.WithName("pullsecret"), c, uncachedC, addon, addonPullSecretKey)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, resultNil, result)
	assert.NotNil(t, secret)
}

func Test_getReferencedPullSecret_retry(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{},
		Spec: addonsv1alpha1.AddonSpec{
			Namespaces: []addonsv1alpha1.AddonNamespace{
				{Name: "test"},
			},
			Install: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						PullSecretName:     "lorem-ipsum",
						CatalogSourceImage: "xxx",
						Namespace:          "test",
					},
				},
			},
		},
	}
	addonPullSecretKey := client.ObjectKey{Name: "lorem-ipsum", Namespace: "xxx-addon-operator"}

	c := testutil.NewClient()
	uncachedC := testutil.NewClient()
	c.
		On("Get", // get referenced Secret
			testutil.IsContext,
			addonPullSecretKey,
			mock.IsType(&corev1.Secret{}),
		).
		Return(testutil.NewTestErrNotFound())
	uncachedC.
		On("Get", // get referenced Secret via uncached read
			testutil.IsContext,
			addonPullSecretKey,
			mock.IsType(&corev1.Secret{}),
		).
		Return(testutil.NewTestErrNotFound())

	r := &AddonReconciler{
		Client:                 c,
		UncachedClient:         uncachedC,
		Log:                    testutil.NewLogger(t),
		Scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		AddonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	secret, result, err := getReferencedSecret(ctx, r.Log.WithName("pullsecret"), c, uncachedC, addon, addonPullSecretKey)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, resultRetry, result)
	assert.Nil(t, secret)

	condition := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.AddonOperatorAvailable)
	if assert.NotNil(t, condition, "available condition should be reported") {
		assert.Equal(t, addonsv1alpha1.AddonReasonMissingSecretForPropagation, condition.Reason)
		assert.Equal(t, metav1.ConditionFalse, condition.Status)
	}
}

func Test_reconcileSecret_CreateWithClientError(t *testing.T) {
	timeoutErr := k8sApiErrors.NewTimeoutError("for testing", 1)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
	}
	key := client.ObjectKey{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}

	c := testutil.NewClient()
	c.On("Get", testutil.IsContext, key, mock.IsType(&corev1.Secret{})).
		Return(timeoutErr)

	ctx := context.Background()
	err := reconcileSecret(ctx, c, secret)
	require.Error(t, err)
	assert.True(t, errors.Is(err, timeoutErr), "is no timeout error")
	c.AssertExpectations(t)
}

func Test_reconcileSecret_Create(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
	}
	key := client.ObjectKey{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}

	c := testutil.NewClient()
	c.
		On("Get", testutil.IsContext, key, mock.IsType(&corev1.Secret{})).
		Return(k8sApiErrors.NewNotFound(schema.GroupResource{}, ""))
	c.
		On("Create", testutil.IsContext, secret, mock.Anything).
		Return(nil)

	ctx := context.Background()
	err := reconcileSecret(ctx, c, secret)
	require.NoError(t, err)

	c.AssertExpectations(t)
}

func Test_reconcileSecret_Update(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
			Labels: map[string]string{
				"test": "test",
			},
		},
	}
	key := client.ObjectKey{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}

	c := testutil.NewClient()
	c.
		On("Get", testutil.IsContext, key, mock.IsType(&corev1.Secret{})).
		Return(nil)
	var updatedSecret *corev1.Secret
	c.
		On("Update", testutil.IsContext, mock.IsType(&corev1.Secret{}), mock.Anything).
		Run(func(args mock.Arguments) {
			updatedSecret = args.Get(1).(*corev1.Secret)
		}).
		Return(nil)

	ctx := context.Background()
	err := reconcileSecret(ctx, c, secret)
	require.NoError(t, err)

	if c.AssertExpectations(t) {
		assert.Equal(t, secret.Labels, updatedSecret.Labels)
	}
}

func TestEnsureSecretPropagation(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "addon-xxx",
		},
		Spec: addonsv1alpha1.AddonSpec{
			Namespaces: []addonsv1alpha1.AddonNamespace{
				{Name: "test"},
			},
			Install: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						CatalogSourceImage: "xxx",
						Namespace:          "test",
					},
				},
			},
			SecretPropagation: &addonsv1alpha1.AddonSecretPropagation{
				Secrets: []addonsv1alpha1.AddonSecretPropagationReference{
					{
						SourceSecret: corev1.LocalObjectReference{
							Name: "src-1",
						},
						DestinationSecret: corev1.LocalObjectReference{
							Name: "dest-1",
						},
					},
				},
			},
		},
	}

	c := testutil.NewClient() // default cached client

	srcSecret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "src-1",
			Namespace: "xxx-addon-operator",
		},
		Data: map[string][]byte{
			"test": []byte("xxx"),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}
	srcSecret1Key := client.ObjectKeyFromObject(srcSecret1)
	c.
		On("Get", testutil.IsContext, srcSecret1Key, mock.IsType(&corev1.Secret{})).
		Run(func(args mock.Arguments) {
			out := args.Get(2).(*corev1.Secret)
			*out = *srcSecret1
		}).
		Return(nil)

	destSecret1Key := client.ObjectKey{
		Name:      "dest-1",
		Namespace: "test",
	}
	c.
		On("Get", testutil.IsContext, destSecret1Key, mock.IsType(&corev1.Secret{})).
		Return(k8sApiErrors.NewNotFound(schema.GroupResource{}, ""))
	var createdDestSecret *corev1.Secret
	c.
		On("Create", testutil.IsContext, mock.IsType(&corev1.Secret{}), mock.Anything).
		Run(func(args mock.Arguments) {
			createdDestSecret = args.Get(1).(*corev1.Secret)
		}).
		Return(nil)

	secretToDelete := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "xxx",
		},
	}
	c.
		On("List", testutil.IsContext, mock.IsType(&corev1.SecretList{}), mock.Anything).
		Run(func(args mock.Arguments) {
			out := args.Get(1).(*corev1.SecretList)
			*out = corev1.SecretList{
				Items: []corev1.Secret{
					{ // secret that should be there and not deleted
						ObjectMeta: metav1.ObjectMeta{
							Name:      destSecret1Key.Name,
							Namespace: destSecret1Key.Namespace,
						},
					},

					// this is some leftover secret on the cluster,
					// it should be deleted
					*secretToDelete,
				},
			}
		}).
		Return(nil)
	c.
		On("Delete", testutil.IsContext, secretToDelete, mock.Anything).
		Return(nil)

	r := &AddonReconciler{
		Client:                 c,
		Log:                    testutil.NewLogger(t),
		Scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		AddonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	result, err := r.ensureSecretPropagation(ctx, r.Log.WithName("pullsecret"), addon)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, resultNil, result)

	if assert.NotNil(t, createdDestSecret) {
		assert.Equal(t, srcSecret1.Type, createdDestSecret.Type)
		assert.Equal(t, map[string]string{
			"app.kubernetes.io/instance":   "addon-xxx",
			"app.kubernetes.io/managed-by": "addon-operator",
		}, createdDestSecret.Labels)
	}
}

func TestEnsureSecretPropagation_cleanup_when_nil(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "addon-xxx",
		},
		Spec: addonsv1alpha1.AddonSpec{
			Namespaces: []addonsv1alpha1.AddonNamespace{
				{Name: "test"},
			},
			Install: addonsv1alpha1.AddonInstallSpec{
				Type: addonsv1alpha1.OLMOwnNamespace,
				OLMOwnNamespace: &addonsv1alpha1.AddonInstallOLMOwnNamespace{
					AddonInstallOLMCommon: addonsv1alpha1.AddonInstallOLMCommon{
						CatalogSourceImage: "xxx",
						Namespace:          "test",
					},
				},
			},
			SecretPropagation: nil,
		},
	}

	c := testutil.NewClient() // default cached client

	secretToDelete := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "xxx",
		},
	}
	c.
		On("List", testutil.IsContext, mock.IsType(&corev1.SecretList{}), mock.Anything).
		Run(func(args mock.Arguments) {
			out := args.Get(1).(*corev1.SecretList)
			*out = corev1.SecretList{
				Items: []corev1.Secret{
					// this is some leftover secret on the cluster,
					// it should be deleted
					*secretToDelete,
				},
			}
		}).
		Return(nil)
	c.
		On("Delete", testutil.IsContext, secretToDelete, mock.Anything).
		Return(nil)

	r := &AddonReconciler{
		Client:                 c,
		Log:                    testutil.NewLogger(t),
		Scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		AddonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	result, err := r.ensureSecretPropagation(ctx, r.Log.WithName("pullsecret"), addon)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, resultNil, result)
}

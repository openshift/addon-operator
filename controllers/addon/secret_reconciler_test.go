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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
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
	c.On("Get", mock.Anything,
		testutil.IsObjectKey,
		mock.IsType(&corev1.Secret{}),
		mock.Anything,
		mock.Anything).
		Return(timeoutErr)

	ctx := context.Background()
	err := reconcileSecret(ctx, c, secret)
	require.Error(t, err)
	require.ErrorIs(t, err, timeoutErr)
	c.AssertExpectations(t)
	c.AssertCalled(t, "Get", mock.Anything, client.ObjectKey{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}, mock.IsType(&corev1.Secret{}), mock.Anything)
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
			mock.Anything,
			addonPullSecretKey,
			mock.IsType(&corev1.Secret{}),
			mock.Anything,
		).
		Return(testutil.NewTestErrNotFound())
	uncachedC.
		On("Get", // get referenced Secret via uncached read
			mock.Anything,
			addonPullSecretKey,
			mock.IsType(&corev1.Secret{}),
			mock.Anything,
		).
		Return(nil)
	c.On("Patch", // patch referenced Secret to add it to cache
		mock.Anything,
		mock.IsType(&corev1.Secret{}),
		mock.Anything,
		mock.Anything,
	).Return(nil)

	r := &addonSecretPropagationReconciler{
		cachedClient:           c,
		uncachedClient:         uncachedC,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	secret, result, err := r.getReferencedSecret(ctx, addon, addonPullSecretKey)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // empty reconcile result
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
			mock.Anything,
			addonPullSecretKey,
			mock.IsType(&corev1.Secret{}),
			mock.Anything,
		).
		Return(testutil.NewTestErrNotFound())
	uncachedC.
		On("Get", // get referenced Secret via uncached read
			mock.Anything,
			addonPullSecretKey,
			mock.IsType(&corev1.Secret{}),
			mock.Anything,
		).
		Return(testutil.NewTestErrNotFound())

	r := &addonSecretPropagationReconciler{
		cachedClient:           c,
		uncachedClient:         uncachedC,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	secret, result, err := r.getReferencedSecret(ctx, addon, addonPullSecretKey)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{
		RequeueAfter: defaultRetryAfterTime,
	}, result) // retry
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
	c.On("Get", mock.Anything, key, mock.IsType(&corev1.Secret{}), mock.Anything).
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
		On("Get", mock.Anything, key, mock.IsType(&corev1.Secret{}), mock.Anything).
		Return(k8sApiErrors.NewNotFound(schema.GroupResource{}, ""))
	c.
		On("Create", mock.Anything, secret, mock.Anything).
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
		On("Get", mock.Anything, key, mock.IsType(&corev1.Secret{}), mock.Anything).
		Return(nil)
	var updatedSecret *corev1.Secret
	c.
		On("Update", mock.Anything, mock.IsType(&corev1.Secret{}), mock.Anything).
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
		On("Get", mock.Anything, srcSecret1Key, mock.IsType(&corev1.Secret{}), mock.Anything).
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
		On("Get", mock.Anything, destSecret1Key, mock.IsType(&corev1.Secret{}), mock.Anything).
		Return(k8sApiErrors.NewNotFound(schema.GroupResource{}, ""))
	var createdDestSecret *corev1.Secret
	c.
		On("Create", mock.Anything, mock.IsType(&corev1.Secret{}), mock.Anything).
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
		On("List", mock.Anything, mock.IsType(&corev1.SecretList{}), mock.Anything).
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
		On("Delete", mock.Anything, secretToDelete, mock.Anything).
		Return(nil)

	r := &addonSecretPropagationReconciler{
		cachedClient:           c,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := controllers.ContextWithLogger(context.Background(), testutil.NewLogger(t))
	result, err := r.Reconcile(ctx, addon)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	if assert.NotNil(t, createdDestSecret) {
		assert.Equal(t, srcSecret1.Type, createdDestSecret.Type)
		assert.Equal(t, map[string]string{
			controllers.CommonInstanceLabel:  "addon-xxx",
			controllers.CommonManagedByLabel: controllers.CommonManagedByValue,
			controllers.CommonCacheLabel:     controllers.CommonCacheValue,
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
		On("List", mock.Anything, mock.IsType(&corev1.SecretList{}), mock.Anything).
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
		On("Delete", mock.Anything, secretToDelete, mock.Anything).
		Return(nil)

	r := &addonSecretPropagationReconciler{
		cachedClient:           c,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}
	ctx := controllers.ContextWithLogger(context.Background(), testutil.NewLogger(t))
	result, err := r.Reconcile(ctx, addon)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestGetDestinationSecretWithoutNamespace_NoSecrets(t *testing.T) {
	type Expected struct {
		secret []corev1.Secret
		result ctrl.Result
		err    error
	}
	testCases := map[string]struct {
		addon    *addonsv1alpha1.Addon
		expected Expected
	}{
		"SecretPropagation is nil": {
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					SecretPropagation: nil,
				},
			},
			expected: Expected{
				secret: nil,
				result: ctrl.Result{},
				err:    nil,
			},
		},
		"Secrets is empty": {
			addon: &addonsv1alpha1.Addon{
				Spec: addonsv1alpha1.AddonSpec{
					SecretPropagation: &addonsv1alpha1.AddonSecretPropagation{
						Secrets: []addonsv1alpha1.AddonSecretPropagationReference{},
					},
				},
			},
			expected: Expected{
				secret: nil,
				result: ctrl.Result{},
				err:    nil,
			},
		},
	}

	c := testutil.NewClient()
	uncachedC := testutil.NewClient()

	c.
		On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(nil)
	uncachedC.
		On("Get", testutil.IsContext, testutil.IsObjectKey, testutil.IsCoreV1NamespacePtr).Return(nil)

	r := &addonSecretPropagationReconciler{
		cachedClient:           c,
		uncachedClient:         uncachedC,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	for key, tc := range testCases {
		t.Run(key, func(t *testing.T) {
			addon := tc.addon.DeepCopy()
			secret, result, err := r.getDestinationSecretsWithoutNamespace(ctx, addon)
			assert.Equal(t, tc.expected.secret, secret)
			assert.Equal(t, tc.expected.result, result)
			assert.Equal(t, tc.expected.err, err)
		})
	}
}

func TestGetDestinationSecretWithoutNamespace_WithSecrets(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "addon-mock",
		},
		Spec: addonsv1alpha1.AddonSpec{
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
	secretKey := client.ObjectKey{Name: "src-1", Namespace: "xxx-addon-operator"}

	c := testutil.NewClient()
	uncachedC := testutil.NewClient()
	c.
		On("Get",
			mock.Anything,
			secretKey,
			mock.IsType(&corev1.Secret{}),
			mock.Anything,
		).
		Return(testutil.NewTestErrNotFound())
	uncachedC.
		On("Get",
			mock.Anything,
			secretKey,
			mock.IsType(&corev1.Secret{}),
			mock.Anything,
		).
		Return(testutil.NewTestErrNotFound())

	ctx := context.Background()

	r := &addonSecretPropagationReconciler{
		cachedClient:           c,
		uncachedClient:         uncachedC,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	secret, result, err := r.getDestinationSecretsWithoutNamespace(ctx, addon)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{
		RequeueAfter: defaultRetryAfterTime,
	}, result)
	assert.Nil(t, secret)
}

func TestGetDestinationSecretWithoutNamespace_WithSecretsUncachedFallback(t *testing.T) {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: "addon-mock",
		},
		Spec: addonsv1alpha1.AddonSpec{
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
	secretKey := client.ObjectKey{Name: "src-1", Namespace: "xxx-addon-operator"}

	c := testutil.NewClient()
	uncachedC := testutil.NewClient()

	c.
		On("Get",
			mock.Anything,
			secretKey,
			mock.IsType(&corev1.Secret{}),
			mock.Anything,
		).
		Return(testutil.NewTestErrNotFound())
	uncachedC.
		On("Get",
			mock.Anything,
			secretKey,
			mock.IsType(&corev1.Secret{}),
			mock.Anything,
		).
		Return(nil)
	c.On("Patch",
		mock.Anything,
		mock.IsType(&corev1.Secret{}),
		mock.Anything,
		mock.Anything,
	).Return(nil)

	r := &addonSecretPropagationReconciler{
		cachedClient:           c,
		uncachedClient:         uncachedC,
		scheme:                 testutil.NewTestSchemeWithAddonsv1alpha1(),
		addonOperatorNamespace: "xxx-addon-operator",
	}

	ctx := context.Background()
	secret, result, err := r.getDestinationSecretsWithoutNamespace(ctx, addon)
	c.AssertExpectations(t)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.NotNil(t, secret)
}

// TestAddonSecretPropagationReconciler_Name verifies that the method returns the expected string value.
// It ensures that the Name method is correctly implemented and that it consistently returns the predefined name.
func TestAddonSecretPropagationReconciler_Name(t *testing.T) {

	r := &addonSecretPropagationReconciler{}

	expectedName := "secretPropogationReconciler"

	name := r.Name()

	assert.Equal(t, expectedName, name)
}

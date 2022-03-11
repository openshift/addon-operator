package addon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8sApiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestEnsureSecretPropagation_NoOp(t *testing.T) {
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
						// PullSecretName:     "lorem-ipsum", Not set -> no op
						CatalogSourceImage: "xxx",
						Namespace:          "test",
					},
				},
			},
		},
	}

	c := testutil.NewClient() // default cached client

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

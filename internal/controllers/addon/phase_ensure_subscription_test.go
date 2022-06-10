package addon

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureSubscription_Adoption(t *testing.T) {
	subscription := testutil.NewTestSubscription()

	c := testutil.NewClient()
	c.On("Get",
		testutil.IsContext,
		testutil.IsObjectKey,
		testutil.IsOperatorsV1Alpha1SubscriptionPtr,
	).Run(func(args mock.Arguments) {
		currentSubscription := testutil.NewTestSubscriptionWithoutOwner()
		currentSubscription.DeepCopyInto(args.Get(2).(*operatorsv1alpha1.Subscription))
	}).Return(nil)

	c.On("Update",
		testutil.IsContext,
		testutil.IsOperatorsV1Alpha1SubscriptionPtr,
		mock.Anything,
	).Return(nil)

	rec := olmReconciler{
		client: c,
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
	}

	ctx := context.Background()
	reconciledSubscription, err := rec.reconcileSubscription(ctx, subscription.DeepCopy())

	assert.NoError(t, err)
	assert.NotNil(t, reconciledSubscription)
	assert.True(t, equality.Semantic.DeepEqual(subscription.OwnerReferences, reconciledSubscription.OwnerReferences))
	c.AssertExpectations(t)

}

func TestCreateSubscriptionConfigObject(t *testing.T) {
	testCases := []struct {
		commonOLMInstallOptions    addonsv1alpha1.AddonInstallOLMCommon
		expectedSubscriptionConfig *operatorsv1alpha1.SubscriptionConfig
	}{
		{
			commonOLMInstallOptions: addonsv1alpha1.AddonInstallOLMCommon{
				Config: nil,
			},
			expectedSubscriptionConfig: nil,
		},
		{
			commonOLMInstallOptions: addonsv1alpha1.AddonInstallOLMCommon{
				Config: &addonsv1alpha1.SubscriptionConfig{
					EnvironmentVariables: []addonsv1alpha1.EnvObject{
						{
							Name:  "test",
							Value: "test",
						},
					},
				},
			},
			expectedSubscriptionConfig: &operatorsv1alpha1.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "test",
						Value: "test",
					},
				},
			},
		},
		{
			commonOLMInstallOptions: addonsv1alpha1.AddonInstallOLMCommon{
				Config: &addonsv1alpha1.SubscriptionConfig{
					EnvironmentVariables: []addonsv1alpha1.EnvObject{
						{
							Name:  "test-1",
							Value: "test-1",
						},
						{
							Name:  "test-2",
							Value: "test-2",
						},
					},
				},
			},
			expectedSubscriptionConfig: &operatorsv1alpha1.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "test-1",
						Value: "test-1",
					},
					{
						Name:  "test-2",
						Value: "test-2",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run("create subscription config object test", func(t *testing.T) {
			subscriptionConfig := createSubscriptionConfigObject(tc.commonOLMInstallOptions)
			assert.Equal(t, tc.expectedSubscriptionConfig, subscriptionConfig)
		})
	}
}

func TestGetSubscriptionEnvObjects(t *testing.T) {
	testCases := []struct {
		envObjects   []addonsv1alpha1.EnvObject
		expectedEnvs []corev1.EnvVar
	}{
		{
			envObjects:   []addonsv1alpha1.EnvObject{},
			expectedEnvs: []corev1.EnvVar{},
		},
		{
			envObjects: []addonsv1alpha1.EnvObject{
				{
					Name:  "test",
					Value: "test",
				},
			},
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "test",
					Value: "test",
				},
			},
		},
		{
			envObjects: []addonsv1alpha1.EnvObject{
				{
					Name:  "test-1",
					Value: "test-1",
				},
				{
					Name:  "test-2",
					Value: "test-2",
				},
			},
			expectedEnvs: []corev1.EnvVar{
				{
					Name:  "test-1",
					Value: "test-1",
				},
				{
					Name:  "test-2",
					Value: "test-2",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run("get subscription environment objects test", func(t *testing.T) {
			subscriptionEnvObjects := getSubscriptionEnvObjects(tc.envObjects)
			assert.Equal(t, tc.expectedEnvs, subscriptionEnvObjects)
		})
	}
}

package addon

import (
	"context"
	"testing"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureSubscription_Adoption(t *testing.T) {
	for name, tc := range map[string]struct {
		MustAdopt  bool
		Strategy   addonsv1alpha1.ResourceAdoptionStrategyType
		AssertFunc func(*testing.T, *operatorsv1alpha1.Subscription, error)
	}{
		"no strategy/no adoption": {
			MustAdopt:  false,
			Strategy:   addonsv1alpha1.ResourceAdoptionStrategyType(""),
			AssertFunc: assertReconciledSubscription,
		},
		"Prevent/no adoption": {
			MustAdopt:  false,
			Strategy:   addonsv1alpha1.ResourceAdoptionPrevent,
			AssertFunc: assertReconciledSubscription,
		},
		"AdoptAll/no adoption": {
			MustAdopt:  false,
			Strategy:   addonsv1alpha1.ResourceAdoptionAdoptAll,
			AssertFunc: assertReconciledSubscription,
		},
		"no strategy/must adopt": {
			MustAdopt:  true,
			Strategy:   addonsv1alpha1.ResourceAdoptionStrategyType(""),
			AssertFunc: assertUnreconciledSubscription,
		},
		"Prevent/must adopt": {
			MustAdopt:  true,
			Strategy:   addonsv1alpha1.ResourceAdoptionPrevent,
			AssertFunc: assertUnreconciledSubscription,
		},
		"AdoptAll/must adopt": {
			MustAdopt:  true,
			Strategy:   addonsv1alpha1.ResourceAdoptionAdoptAll,
			AssertFunc: assertReconciledSubscription,
		},
	} {
		t.Run(name, func(t *testing.T) {
			subscription := testutil.NewTestSubscription()

			c := testutil.NewClient()
			c.On("Get",
				testutil.IsContext,
				testutil.IsObjectKey,
				testutil.IsOperatorsV1Alpha1SubscriptionPtr,
			).Run(func(args mock.Arguments) {
				var sub *operatorsv1alpha1.Subscription

				if tc.MustAdopt {
					sub = testutil.NewTestSubscriptionWithoutOwner()
				} else {
					sub = testutil.NewTestSubscription()
					// Unrelated spec change to force reconciliation
					sub.Spec.Channel = "alpha"
				}

				sub.DeepCopyInto(args.Get(2).(*operatorsv1alpha1.Subscription))
			}).Return(nil)

			if !tc.MustAdopt || (tc.MustAdopt && tc.Strategy == addonsv1alpha1.ResourceAdoptionAdoptAll) {
				c.On("Update",
					testutil.IsContext,
					testutil.IsOperatorsV1Alpha1SubscriptionPtr,
					mock.Anything,
				).Return(nil)
			}

			rec := olmReconciler{
				client: c,
				scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
			}

			ctx := context.Background()
			reconciledSubscription, err := rec.reconcileSubscription(ctx, subscription.DeepCopy(), tc.Strategy)

			tc.AssertFunc(t, reconciledSubscription, err)
			c.AssertExpectations(t)
		})
	}
}

func assertReconciledSubscription(t *testing.T, sub *operatorsv1alpha1.Subscription, err error) {
	t.Helper()

	assert.NoError(t, err)
	assert.NotNil(t, sub)

}

func assertUnreconciledSubscription(t *testing.T, sub *operatorsv1alpha1.Subscription, err error) {
	t.Helper()

	assert.Error(t, err)
	assert.EqualError(t, err, controllers.ErrNotOwnedByUs.Error())
}

func TestCreateSubscriptionObject(t *testing.T) {
	testCases := []struct {
		commonInstallOptions addonsv1alpha1.AddonInstallOLMCommon
		expected             *operatorsv1alpha1.SubscriptionConfig
	}{
		{
			commonInstallOptions: addonsv1alpha1.AddonInstallOLMCommon{
				Config: nil,
			},
			expected: nil,
		},
		{
			commonInstallOptions: addonsv1alpha1.AddonInstallOLMCommon{
				Config: &addonsv1alpha1.SubscriptionConfig{
					EnvironmentVariables: []addonsv1alpha1.EnvObject{
						{
							Name:  "test",
							Value: "test",
						},
					},
				},
			},
			expected: &operatorsv1alpha1.SubscriptionConfig{
				Env: []corev1.EnvVar{
					{
						Name:  "test",
						Value: "test",
					},
				},
			},
		},
		{
			commonInstallOptions: addonsv1alpha1.AddonInstallOLMCommon{
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
			expected: &operatorsv1alpha1.SubscriptionConfig{
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
			subscriptionConfig := createSubscriptionConfigObject(tc.commonInstallOptions)
			assert.Equal(t, tc.expected, subscriptionConfig)
		})
	}
}

func TestGetSubscriptionEnvObjects(t *testing.T) {
	testCases := []struct {
		envObjects []addonsv1alpha1.EnvObject
		expected   []corev1.EnvVar
	}{
		{
			envObjects: []addonsv1alpha1.EnvObject{},
			expected:   []corev1.EnvVar{},
		},
		{
			envObjects: []addonsv1alpha1.EnvObject{
				{
					Name:  "test",
					Value: "test",
				},
			},
			expected: []corev1.EnvVar{
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
			expected: []corev1.EnvVar{
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
			assert.Equal(t, tc.expected, subscriptionEnvObjects)
		})
	}
}

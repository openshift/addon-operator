package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	av1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestAddonInstanceClientImplInterfaces(t *testing.T) {
	t.Parallel()

	require.Implements(t, new(AddonInstanceClient), new(AddonInstanceClientImpl))
}

func TestAddonInstanceClientImplSendPulse(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		Instance   av1alpha1.AddonInstance
		Conditions []metav1.Condition
	}{
		"no conditions": {
			Instance: av1alpha1.AddonInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      av1alpha1.DefaultAddonInstanceName,
					Namespace: "test-namespace",
				},
			},
		},
		"single condition": {
			Instance: av1alpha1.AddonInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      av1alpha1.DefaultAddonInstanceName,
					Namespace: "test-namespace",
				},
			},
			Conditions: []metav1.Condition{
				NewAddonInstanceConditionInstalled(
					metav1.ConditionTrue,
					av1alpha1.AddonInstanceInstalledReasonSetupComplete,
					"All components up",
				),
			},
		},
		"multiple conditions": {
			Instance: av1alpha1.AddonInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      av1alpha1.DefaultAddonInstanceName,
					Namespace: "test-namespace",
				},
			},
			Conditions: []metav1.Condition{
				NewAddonInstanceConditionInstalled(
					metav1.ConditionFalse,
					av1alpha1.AddonInstanceInstalledReasonSetupComplete,
					"All components up",
				),
				NewAddonInstanceConditionDegraded(
					metav1.ConditionTrue,
					"ServiceXUnavailable",
					"Service X database is unreachable",
				),
				NewAddonInstanceConditionReadyToBeDeleted(
					metav1.ConditionTrue,
					av1alpha1.AddonInstanceReasonReadyToBeDeleted,
					"Ready to be deleted",
				),
			},
		},
	} {
		tc := tc

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			scheme := runtime.NewScheme()
			require.NoError(t, av1alpha1.AddToScheme(scheme))

			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&tc.Instance).
				WithObjects(&tc.Instance).
				Build()

			aiClient := NewAddonInstanceClient(c)

			require.NoError(t, aiClient.SendPulse(ctx, tc.Instance, WithConditions(tc.Conditions)))

			var updatedInstance av1alpha1.AddonInstance
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(&tc.Instance), &updatedInstance))

			assert.NotZero(t, updatedInstance.Status.LastHeartbeatTime)
			testutil.AssertConditionsMatch(t, tc.Conditions, updatedInstance.Status.Conditions)
		})
	}
}

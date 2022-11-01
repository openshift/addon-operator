package addoninstance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	av1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers/addoninstance"
	"github.com/openshift/addon-operator/internal/controllers/addoninstance/internal/phase"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestControllerInterface(t *testing.T) {
	t.Parallel()

	require.Implements(t, new(reconcile.Reconciler), new(addoninstance.Controller))
}

func TestController(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		Request    phase.Request
		Result     phase.Result
		ShouldFail bool
	}{
		"phase success/single status condition": {
			Request: phase.Request{
				Instance: av1alpha1.AddonInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      av1alpha1.DefaultAddonInstanceName,
						Namespace: "test-namespace",
					},
					Spec: av1alpha1.AddonInstanceSpec{
						HeartbeatUpdatePeriod: metav1.Duration{
							Duration: av1alpha1.DefaultAddonInstanceHeartbeatUpdatePeriod,
						},
					},
				},
			},
			Result: phase.Success(
				metav1.Condition{
					Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
					Status:  "Unknown",
					Reason:  av1alpha1.AddonInstanceHealthyReasonPendingFirstHeartbeat.String(),
					Message: "Waiting for first heartbeat.",
				},
			),
			ShouldFail: false,
		},
		"phase success/multiple status conditions": {
			Request: phase.Request{
				Instance: av1alpha1.AddonInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      av1alpha1.DefaultAddonInstanceName,
						Namespace: "test-namespace",
					},
					Spec: av1alpha1.AddonInstanceSpec{
						HeartbeatUpdatePeriod: metav1.Duration{
							Duration: av1alpha1.DefaultAddonInstanceHeartbeatUpdatePeriod,
						},
					},
				},
			},
			Result: phase.Success(
				metav1.Condition{
					Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
					Status:  "Unknown",
					Reason:  av1alpha1.AddonInstanceHealthyReasonPendingFirstHeartbeat.String(),
					Message: "Waiting for first heartbeat.",
				},
				metav1.Condition{
					Type:    av1alpha1.AddonInstanceConditionDegraded.String(),
					Status:  "True",
					Reason:  "ServiceXUnavailable",
					Message: "Service X database is unreachable.",
				},
			),
			ShouldFail: false,
		},
		"phase failure": {
			Request: phase.Request{
				Instance: av1alpha1.AddonInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      av1alpha1.DefaultAddonInstanceName,
						Namespace: "test-namespace",
					},
					Spec: av1alpha1.AddonInstanceSpec{
						HeartbeatUpdatePeriod: metav1.Duration{
							Duration: av1alpha1.DefaultAddonInstanceHeartbeatUpdatePeriod,
						},
					},
				},
			},
			Result:     phase.Error(errors.New("test error")),
			ShouldFail: true,
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
				WithObjects(&tc.Request.Instance).
				Build()

			var mPhase PhaseMock

			mPhase.
				On("Execute", mock.Anything, mock.AnythingOfType("phase.Request")).
				Return(tc.Result)

			mPhase.
				On("String").
				Return("PhaseMock")

			aiCtrl := addoninstance.NewController(c,
				addoninstance.WithSerialPhases{
					&mPhase,
				},
			)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      av1alpha1.DefaultAddonInstanceName,
					Namespace: "test-namespace",
				},
			}

			_, err := aiCtrl.Reconcile(ctx, req)
			if tc.ShouldFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			var updatedInstance av1alpha1.AddonInstance
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(&tc.Request.Instance), &updatedInstance))

			if tc.ShouldFail {
				//Conditions are unchanged
				testutil.AssertConditionsMatch(t, tc.Request.Instance.Status.Conditions, updatedInstance.Status.Conditions)
			} else {
				//Conditions are updataed
				testutil.AssertConditionsMatch(t, tc.Result.Conditions, updatedInstance.Status.Conditions)
			}
		})
	}
}

type PhaseMock struct {
	mock.Mock
}

func (m *PhaseMock) Execute(ctx context.Context, req phase.Request) phase.Result {
	args := m.Called(ctx, req)

	return args.Get(0).(phase.Result)
}

func (m *PhaseMock) String() string {
	args := m.Called()

	return args.String(0)
}

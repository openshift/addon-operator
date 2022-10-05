package addoninstance

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	av1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers/addoninstance/internal/phase"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestPhaseCheckHeartbeatInterface(t *testing.T) {
	t.Parallel()

	require.Implements(t, new(Phase), new(PhaseCheckHeartbeat))
}

func TestPhaseCheckHeartbeatExecute(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		Request           phase.Request
		ExpectedConditons []metav1.Condition
	}{
		"heartbeat present, within threshold, and already healthy": {
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
					Status: av1alpha1.AddonInstanceStatus{
						LastHeartbeatTime: metav1.NewTime(timeFixture().Add(-25 * time.Second)),
						Conditions: []metav1.Condition{
							{
								Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
								Status:  "True",
								Reason:  av1alpha1.AddonInstanceHealthyReasonReceivingHeartbeats.String(),
								Message: conditionHealthyMessageHeartbeatReceived,
							},
						},
					},
				},
			},
			ExpectedConditons: []metav1.Condition{
				{
					Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
					Status:  "True",
					Reason:  av1alpha1.AddonInstanceHealthyReasonReceivingHeartbeats.String(),
					Message: conditionHealthyMessageHeartbeatReceived,
				},
			},
		},
		"heartbeat present and within threshold": {
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
					Status: av1alpha1.AddonInstanceStatus{
						LastHeartbeatTime: metav1.NewTime(timeFixture().Add(-25 * time.Second)),
					},
				},
			},
			ExpectedConditons: []metav1.Condition{
				{
					Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
					Status:  "True",
					Reason:  av1alpha1.AddonInstanceHealthyReasonReceivingHeartbeats.String(),
					Message: conditionHealthyMessageHeartbeatReceived,
				},
			},
		},
		"heartbeat present and not within threshold": {
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
					Status: av1alpha1.AddonInstanceStatus{
						LastHeartbeatTime: metav1.NewTime(timeFixture().Add(-35 * time.Second)),
					},
				},
			},
			ExpectedConditons: []metav1.Condition{
				{
					Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
					Status:  "Unknown",
					Reason:  av1alpha1.AddonInstanceHealthyReasonHeartbeatTimeout.String(),
					Message: conditionHealthyMessageHeartbeatNotReceived,
				},
			},
		},
		"heartbeat not present": {
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
			ExpectedConditons: []metav1.Condition{
				{
					Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
					Status:  "Unknown",
					Reason:  av1alpha1.AddonInstanceHealthyReasonPendingFirstHeartbeat.String(),
					Message: conditionHealthyMessageWaiting,
				},
			},
		},
	} {
		tc := tc

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			var clock ClockMock

			clock.
				On("Now").
				Return(timeFixture())

			phase := NewPhaseCheckHeartbeat(
				WithClock{Clock: &clock},
			)

			res := phase.Execute(ctx, tc.Request)
			require.NoError(t, res.Error())

			testutil.AssertConditionsMatch(t, tc.ExpectedConditons, res.Conditions)
		})
	}
}

type ClockMock struct {
	mock.Mock
}

func (c *ClockMock) Now() time.Time {
	args := c.Called()

	return args.Get(0).(time.Time)
}

func timeFixture() time.Time {
	return time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
}

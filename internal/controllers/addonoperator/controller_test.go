package addonoperator

import (
	"context"
	"testing"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestHandleAddonOperatorPause_(t *testing.T) {
	t.Run("enables global pause", func(t *testing.T) {
		c := testutil.NewClient()
		gpm := &globalPauseManagerMock{}
		r := &AddonOperatorReconciler{
			Client:             c,
			GlobalPauseManager: gpm,
		}
		ctx := context.Background()
		ao := &addonsv1alpha1.AddonOperator{
			Spec: addonsv1alpha1.AddonOperatorSpec{
				Paused: true,
			},
		}

		gpm.On("EnableGlobalPause", mock.Anything).Return(nil)
		c.StatusMock.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		err := r.handleGlobalPause(ctx, ao)
		require.NoError(t, err)

		gpm.AssertCalled(t, "EnableGlobalPause", mock.Anything)

		pausedCond := meta.FindStatusCondition(ao.Status.Conditions, addonsv1alpha1.Paused)
		if assert.NotNil(t, pausedCond, "Paused condition should be present on AddonOperator object") {
			assert.Equal(t, metav1.ConditionTrue, pausedCond.Status)
		}
	})

	t.Run("does not enable pause twice when status is already reported", func(t *testing.T) {
		c := testutil.NewClient()
		gpm := &globalPauseManagerMock{}
		r := &AddonOperatorReconciler{
			Client:             c,
			GlobalPauseManager: gpm,
		}
		ctx := context.Background()
		ao := &addonsv1alpha1.AddonOperator{
			Spec: addonsv1alpha1.AddonOperatorSpec{
				Paused: true,
			},
			Status: addonsv1alpha1.AddonOperatorStatus{
				Conditions: []metav1.Condition{
					{
						Type:   addonsv1alpha1.Paused,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		gpm.On("EnableGlobalPause", mock.Anything).Return(nil)
		c.StatusMock.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		err := r.handleGlobalPause(ctx, ao)
		require.NoError(t, err)

		// When status is already reported, don't EnableGlobalPause again.
		gpm.AssertNotCalled(t, "EnableGlobalPause", mock.Anything)
	})

	t.Run("disables global pause", func(t *testing.T) {
		c := testutil.NewClient()
		gpm := &globalPauseManagerMock{}
		r := &AddonOperatorReconciler{
			Client:             c,
			GlobalPauseManager: gpm,
		}
		ctx := context.Background()
		ao := &addonsv1alpha1.AddonOperator{
			Spec: addonsv1alpha1.AddonOperatorSpec{
				Paused: false,
			},
			Status: addonsv1alpha1.AddonOperatorStatus{
				Conditions: []metav1.Condition{
					{
						Type:   addonsv1alpha1.Paused,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		gpm.On("DisableGlobalPause", mock.Anything).Return(nil)
		c.StatusMock.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		err := r.handleGlobalPause(ctx, ao)
		require.NoError(t, err)

		gpm.AssertCalled(t, "DisableGlobalPause", mock.Anything)
		pausedCond := meta.FindStatusCondition(ao.Status.Conditions, addonsv1alpha1.Paused)
		assert.Nil(t, pausedCond, "Paused condition should be removed on AddonOperator object")
	})

	t.Run("does not disable twice when status is already reported", func(t *testing.T) {
		c := testutil.NewClient()
		gpm := &globalPauseManagerMock{}
		r := &AddonOperatorReconciler{
			Client:             c,
			GlobalPauseManager: gpm,
		}
		ctx := context.Background()
		ao := &addonsv1alpha1.AddonOperator{
			Spec: addonsv1alpha1.AddonOperatorSpec{
				Paused: false,
			},
		}

		gpm.On("DisableGlobalPause", mock.Anything).Return(nil)
		c.StatusMock.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		err := r.handleGlobalPause(ctx, ao)
		require.NoError(t, err)

		// When status is gone, don't DisableGlobalPause again.
		gpm.AssertNotCalled(t, "DisableGlobalPause", mock.Anything)
	})
}

type globalPauseManagerMock struct {
	mock.Mock
}

func (r *globalPauseManagerMock) EnableGlobalPause(ctx context.Context) error {
	args := r.Called(ctx)
	return args.Error(0)
}

func (r *globalPauseManagerMock) DisableGlobalPause(ctx context.Context) error {
	args := r.Called(ctx)
	return args.Error(0)
}

// The TestAreSlicesEquivalent function tests the areSlicesEquivalent function
// to verify whether it correctly determines if two slices are equivalent.
func TestAreSlicesEquivalent(t *testing.T) {
	testCases := []struct {
		sliceA         []string
		sliceB         []string
		expectedResult bool
	}{
		// Equivalent addon slices
		{
			sliceA:         []string{"prometheus", "grafana", "rhods-dashboard"},
			sliceB:         []string{"prometheus", "grafana", "rhods-dashboard"},
			expectedResult: true,
		},
		// Non-equivalent addon slices
		{
			sliceA:         []string{"prometheus", "grafana", "rhods-dashboard"},
			sliceB:         []string{"prometheus", "grafana", "redhat-rhoam-operator"},
			expectedResult: false,
		},
		// Slices of different lengths
		{
			sliceA:         []string{"prometheus", "grafana"},
			sliceB:         []string{"grafana", "prometheus", "redhat-rhoam-operator"},
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		result := areSlicesEquivalent(tc.sliceA, tc.sliceB)
		assert.Equal(t, tc.expectedResult, result, "Unexpected result for slices %v and %v", tc.sliceA, tc.sliceB)
	}
}

// The TestEnqueueAddonOperator tests the behavior of the enqueueAddonOperator function.
// The enqueueAddonOperator function enqueues (adds an item of data awaiting processing to a queue)
// a reconcile request for the default addon operator.
func TestEnqueueAddonOperator(t *testing.T) {
	ctx := context.Background()
	q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	expectedRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: addonsv1alpha1.DefaultAddonOperatorName,
		},
	}

	err := enqueueAddonOperator(ctx, &handler.EnqueueRequestForObject{}, q)
	assert.NoError(t, err, "Expected no error")

	// Check that a single request was added to the queue
	assert.Equal(t, 1, q.Len(), "Expected 1 item in the queue")

	// Retrieve the added request from the queue
	item, _ := q.Get()
	request, ok := item.(reconcile.Request)
	assert.True(t, ok, "Expected item to be of type reconcile.Request")
	assert.Equal(t, expectedRequest, request, "Expected request does not match the added request")
}

// The TestAccessTokenFromDockerConfig parses a Docker config JSON, extracts the
// access token associated with the key and returns the token or error, if any failures
// occurs during the process.
func TestAccessTokenFromDockerConfig(t *testing.T) {
	testCases := []struct {
		name       string
		dockerJSON []byte
		expected   string
		expectedErr string
	}{
		{
			name: "ValidDockerConfig",
			dockerJSON: []byte(`{
				"auths": {
					"cloud.openshift.com": {
						"auth": "THIS_IS_AN_API_TOKEN"
					}
				}
			}`),
			expected: "THIS_IS_AN_API_TOKEN",
		},
		{
			name:       "InvalidJSON",
			dockerJSON: []byte(`{invalid JSON}`),
			expectedErr: "unmarshalling docker config json",
		},
		{
			name: "MissingToken",
			dockerJSON: []byte(`{
				"auths": {
					"cloud.openshift.com": {}
				}
			}`),
			expectedErr: "missing token for cloud.openshift.com",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			accessToken, err := accessTokenFromDockerConfig(tc.dockerJSON)

			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, accessToken)
			}
		})
	}
}



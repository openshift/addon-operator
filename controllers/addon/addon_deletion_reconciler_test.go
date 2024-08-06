package addon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

type testClock struct {
	mock.Mock
}

func (c *testClock) Now() time.Time {
	return c.Called().Get(0).(time.Time)
}

type mockdeletionStrategy struct {
	mock.Mock
}

func (m *mockdeletionStrategy) NotifyAddon(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	args := m.Called(ctx, addon)
	return args.Error(0)
}

func (m *mockdeletionStrategy) AckReceivedFromAddon(ctx context.Context, addon *addonsv1alpha1.Addon) (bool, error) {
	args := m.Called(ctx, addon)
	return args.Get(0).(bool), args.Error(1)
}

func TestAddonDeletionReconciler(t *testing.T) {
	t.Run("NOOP when addon doesnt have the delete annotation", func(t *testing.T) {
		client := testutil.NewClient()

		reconciler := addonDeletionReconciler{
			clock: defaultClock{},
			handlers: []addonDeletionHandler{
				&legacyDeletionHandler{
					client:         client,
					uncachedClient: client,
				},
				&addonInstanceDeletionHandler{client: client},
			},
		}

		res, err := reconciler.Reconcile(context.Background(), &addonsv1alpha1.Addon{})
		require.NoError(t, err)
		require.True(t, res.IsZero())
	})

	t.Run("NOOP when addon has delete annotation and readyToBeDeleted=true status condition", func(t *testing.T) {
		client := testutil.NewClient()
		reconciler := addonDeletionReconciler{
			clock: defaultClock{},
			handlers: []addonDeletionHandler{
				&legacyDeletionHandler{
					client:         client,
					uncachedClient: client,
				},
				&addonInstanceDeletionHandler{client: client},
			},
		}
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: v1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					addonsv1alpha1.DeleteAnnotationFlag:  "",
					addonsv1alpha1.DeleteTimeoutDuration: "5m",
				},
			},
			Status: addonsv1alpha1.AddonStatus{
				Conditions: []v1.Condition{
					{
						Status: v1.ConditionTrue,
						Type:   addonsv1alpha1.ReadyToBeDeleted,
					},
				},
			},
		}

		res, err := reconciler.Reconcile(context.Background(), addon)
		require.NoError(t, err)
		require.True(t, res.IsZero())
	})

	t.Run("immeadiately sets readyToBeDeleted=true if spec.DeleteAckRequired=false", func(t *testing.T) {
		client := testutil.NewClient()
		reconciler := addonDeletionReconciler{
			clock: defaultClock{},
			handlers: []addonDeletionHandler{
				&legacyDeletionHandler{
					client:         client,
					uncachedClient: client,
				},
				&addonInstanceDeletionHandler{client: client},
			},
		}
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: v1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					addonsv1alpha1.DeleteAnnotationFlag:  "",
					addonsv1alpha1.DeleteTimeoutDuration: "5m",
				},
			},
			Spec: addonsv1alpha1.AddonSpec{
				DeleteAckRequired: false,
			},
		}

		res, err := reconciler.Reconcile(context.Background(), addon)
		require.NoError(t, err)
		require.True(t, res.IsZero())

		cond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted)
		require.NotNil(t, cond)
		require.Equal(t, v1.ConditionTrue, cond.Status)
	})

	t.Run("removes deletetimeout condition when ack is finally received", func(t *testing.T) {
		mockStrategy := &mockdeletionStrategy{}
		reconciler := addonDeletionReconciler{
			clock: defaultClock{},
			handlers: []addonDeletionHandler{
				mockStrategy,
			},
		}
		addon := &addonsv1alpha1.Addon{
			ObjectMeta: v1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					addonsv1alpha1.DeleteAnnotationFlag:  "",
					addonsv1alpha1.DeleteTimeoutDuration: "5m",
				},
			},
			Spec: addonsv1alpha1.AddonSpec{
				DeleteAckRequired: true,
			},
			Status: addonsv1alpha1.AddonStatus{
				Conditions: []v1.Condition{
					{
						Type:   addonsv1alpha1.DeleteTimeout,
						Status: v1.ConditionTrue,
					},
				},
			},
		}
		// Setup mock calls.
		mockStrategy.On("NotifyAddon", mock.Anything, mock.Anything).Return(nil)
		mockStrategy.On("AckReceivedFromAddon", mock.Anything, mock.Anything).Return(true, nil)

		res, err := reconciler.Reconcile(context.Background(), addon)
		require.NoError(t, err)
		require.True(t, res.IsZero())

		// Assert deletetimeout cond is removed.
		cond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.DeleteTimeout)
		require.Nil(t, cond)
	})

	t.Run("runs deletion strategies and returns errors and reconcile result correctly.", func(t *testing.T) {
		testCases := []struct {
			testCase                    string
			AckReceivedFromAddon        bool
			OCMTimeoutDuration          string
			CurrentTime                 time.Time
			DeletionTimedOutCondPresent bool
			ExpectedReconcileResult     subReconcilerResult
			HandlerErr                  bool
		}{

			{
				testCase:                "handler returns error",
				HandlerErr:              true,
				AckReceivedFromAddon:    false,
				CurrentTime:             time.Now(),
				ExpectedReconcileResult: resultNil,
			},
			{
				testCase:                "reconcile result should have the right RequeueAfter interval",
				HandlerErr:              false,
				AckReceivedFromAddon:    false,
				CurrentTime:             time.Now(),
				ExpectedReconcileResult: resultRequeueAfter(defaultDeleteTimeoutDuration),
			},
			{
				testCase:                "no handler errors and ack received from addon",
				HandlerErr:              false,
				AckReceivedFromAddon:    true,
				CurrentTime:             time.Now(),
				ExpectedReconcileResult: resultNil,
			},
			{
				testCase:                    "reconcile result should have the right RequeueAfter interval",
				HandlerErr:                  false,
				AckReceivedFromAddon:        false,
				OCMTimeoutDuration:          "5m",
				CurrentTime:                 time.Now(),
				DeletionTimedOutCondPresent: false,
				ExpectedReconcileResult:     resultRequeueAfter(5 * time.Minute),
			},
			{
				testCase:                    "reconcile result should have the right RequeueAfter interval",
				HandlerErr:                  false,
				AckReceivedFromAddon:        false,
				OCMTimeoutDuration:          "5minutes", // time.ParseDuration will fail.
				CurrentTime:                 time.Now(),
				DeletionTimedOutCondPresent: false,
				ExpectedReconcileResult:     resultRequeueAfter(defaultDeleteTimeoutDuration),
			},
			{
				testCase:                    "delete timeout condition should be set",
				HandlerErr:                  false,
				AckReceivedFromAddon:        false,
				OCMTimeoutDuration:          "5m",
				CurrentTime:                 time.Now().Add(10 * time.Minute),
				DeletionTimedOutCondPresent: true,
				ExpectedReconcileResult:     resultNil,
			},
		}

		for _, tc := range testCases {
			t.Logf("running test case: %s \n", tc.testCase)
			mockStrategy := &mockdeletionStrategy{}
			testClock := &testClock{}
			testClock.On("Now").Return(tc.CurrentTime)
			reconciler := addonDeletionReconciler{
				clock: testClock,
				handlers: []addonDeletionHandler{
					mockStrategy,
				},
			}
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: v1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						addonsv1alpha1.DeleteAnnotationFlag: "",
					},
				},
				Spec: addonsv1alpha1.AddonSpec{
					DeleteAckRequired: true,
				},
			}
			if len(tc.OCMTimeoutDuration) != 0 {
				addon.Annotations[addonsv1alpha1.DeleteTimeoutDuration] = tc.OCMTimeoutDuration
			}

			// Setup mock calls
			mockStrategy.On("NotifyAddon", mock.Anything, mock.Anything).Return(nil)
			if tc.HandlerErr {
				mockStrategy.On("AckReceivedFromAddon", mock.Anything, mock.Anything).
					Return(false, errors.New("kubeapi server busy"))
			} else {
				if tc.AckReceivedFromAddon {
					mockStrategy.On("AckReceivedFromAddon", mock.Anything, mock.Anything).
						Return(true, nil)
				} else {
					mockStrategy.On("AckReceivedFromAddon", mock.Anything, mock.Anything).
						Return(false, nil)
				}
			}

			// invoke reconciler
			reconcileRes, reconcileErr := reconciler.Reconcile(context.Background(), addon)

			// assertions
			if tc.HandlerErr {
				require.Error(t, reconcileErr)
			} else {
				require.Equal(t, tc.ExpectedReconcileResult, reconcileRes)
			}
			mockStrategy.AssertExpectations(t)

			readyToBeDeletedCondition := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.ReadyToBeDeleted)

			require.NotNil(t, readyToBeDeletedCondition)

			// If no errors and AckReceivedFromAddon = true
			if !tc.HandlerErr && tc.AckReceivedFromAddon {
				require.Equal(t, v1.ConditionTrue, readyToBeDeletedCondition.Status)
			} else {
				// If any error or AckReceivedFromAddon = false
				require.Equal(t, v1.ConditionFalse, readyToBeDeletedCondition.Status)
			}

			if tc.DeletionTimedOutCondPresent {
				cond := meta.FindStatusCondition(addon.Status.Conditions, addonsv1alpha1.DeleteTimeout)
				require.NotNil(t, cond)
				require.Equal(t, v1.ConditionTrue, cond.Status)
			}
		}
	})

}

// The TestAddonDeletionReconciler_Name function, tests the Name
// method of the addonDeletionReconciler type.
func TestAddonDeletionReconciler_Name(t *testing.T) {
	r := &addonDeletionReconciler{}

	// The expected reconciler name
	expectedName := "deletionReconciler"

	name := r.Name()

	// Verify that the reconciler name is correct
	assert.Equal(t, expectedName, name)
}

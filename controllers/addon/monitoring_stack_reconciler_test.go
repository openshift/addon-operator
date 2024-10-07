package addon

import (
	"context"
	"testing"

	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	monv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"

	"github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestEnsureMonitoringStack_MissingConfig(t *testing.T) {
	for _, tc := range []struct {
		missingMonitoringStackConfig bool
		missingMonitoringFullConfig  bool
	}{
		{
			missingMonitoringFullConfig: true,
		},
		{
			missingMonitoringFullConfig:  false,
			missingMonitoringStackConfig: true,
		},
	} {
		c := testutil.NewClient()
		ctx := context.Background()
		r := &monitoringStackReconciler{
			client: c,
			scheme: testutil.NewTestSchemeWithAddonsv1alpha1AndMsov1alpha1(),
		}

		var addon *v1alpha1.Addon
		if tc.missingMonitoringFullConfig {
			addon = testutil.NewTestAddonWithCatalogSourceImage()
		} else if tc.missingMonitoringStackConfig {
			addon = testutil.NewTestAddonWithCatalogSourceImage()
			addon.Spec.Monitoring = testutil.NewTestAddonWithMonitoringFederation().Spec.Monitoring
		}
		assert.NotNil(t, addon)

		_, err := r.ensureMonitoringStack(ctx, addon)
		if tc.missingMonitoringFullConfig || tc.missingMonitoringStackConfig {
			require.ErrorIs(t, err, errMonitoringStackSpecNotFound)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestEnsureMonitoringStack_MonitoringStackPresentInSpec_NotPresentInCluster(t *testing.T) {
	c := testutil.NewClient()
	r := &monitoringStackReconciler{
		client: c,
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
	}

	addon := testutil.NewTestAddonWithMonitoringStack()
	ctx := context.Background()

	c.On("Get", testutil.IsContext, mock.IsType(types.NamespacedName{}), mock.IsType(&obov1alpha1.MonitoringStack{}), mock.Anything).
		Return(testutil.NewTestErrNotFound())
	c.On("Create", testutil.IsContext, mock.IsType(&obov1alpha1.MonitoringStack{}), mock.Anything).
		Run(func(args mock.Arguments) {
			commonConfig, stop := parseAddonInstallConfig(controllers.LoggerFromContext(ctx), addon)
			assert.False(t, stop)

			assert.Equal(t, commonConfig.Namespace, "addon-1")
		}).
		Return(nil)

	_, err := r.ensureMonitoringStack(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", 1)
	c.AssertNumberOfCalls(t, "Create", 1)
}

func TestEnsureMonitoringStack_MonitoringStackPresentInSpec_PresentInCluster(t *testing.T) {
	c := testutil.NewClient()

	r := &monitoringStackReconciler{
		client: c,
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
	}

	addon := testutil.NewTestAddonWithMonitoringStack()

	ctx := context.Background()
	c.On("Get", testutil.IsContext, mock.IsType(types.NamespacedName{}), mock.IsType(&obov1alpha1.MonitoringStack{}), mock.Anything).
		Return(nil)
	c.On("Update", testutil.IsContext, mock.IsType(&obov1alpha1.MonitoringStack{}), mock.Anything).
		Run(func(args mock.Arguments) {
			commonConfig, stop := parseAddonInstallConfig(controllers.LoggerFromContext(ctx), addon)
			assert.False(t, stop)

			assert.Equal(t, commonConfig.Namespace, "addon-1")
		}).
		Return(nil)

	_, err := r.ensureMonitoringStack(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", 1)
	c.AssertNumberOfCalls(t, "Update", 1)
}

func TestPropagateMonitoringStackStatusToAddon(t *testing.T) {
	testCases := []struct {
		name                               string
		monitoringStackStatusFound         obov1alpha1.MonitoringStackStatus
		expectedAvailableStatusToPropagate bool
		expectedAddonStatusMessage         string
	}{
		{
			name: "available-true-condition",
			monitoringStackStatusFound: obov1alpha1.MonitoringStackStatus{
				Conditions: []obov1alpha1.Condition{
					{
						Type:    obov1alpha1.AvailableCondition,
						Status:  obov1alpha1.ConditionTrue,
						Message: "foo",
					},
				},
			},
			expectedAvailableStatusToPropagate: true,
		},
		{
			name: "available-false-condition",
			monitoringStackStatusFound: obov1alpha1.MonitoringStackStatus{
				Conditions: []obov1alpha1.Condition{
					{
						Type:    obov1alpha1.AvailableCondition,
						Status:  obov1alpha1.ConditionFalse,
						Message: "foo",
					},
				},
			},
			expectedAvailableStatusToPropagate: false,
			expectedAddonStatusMessage:         "MonitoringStack is not ready: MonitoringStack Unavailable: foo",
		},
		{
			name: "available-unknown-condition",
			monitoringStackStatusFound: obov1alpha1.MonitoringStackStatus{
				Conditions: []obov1alpha1.Condition{
					{
						Type:    obov1alpha1.AvailableCondition,
						Status:  obov1alpha1.ConditionUnknown,
						Message: "foo",
					},
				},
			},
			expectedAvailableStatusToPropagate: false,
			expectedAddonStatusMessage:         "MonitoringStack is not ready: MonitoringStack Unavailable: foo",
		},
		{
			name: "available-nonexistent-reconciled-true-condition",
			monitoringStackStatusFound: obov1alpha1.MonitoringStackStatus{
				Conditions: []obov1alpha1.Condition{
					{
						Type:    obov1alpha1.ReconciledCondition,
						Status:  obov1alpha1.ConditionTrue,
						Message: "foo",
					},
				},
			},
			expectedAvailableStatusToPropagate: false,
			expectedAddonStatusMessage:         "MonitoringStack is not ready: MonitoringStack successfully reconciled: Pending MonitoringStack to be Available",
		},
		{
			name: "available-nonexistent-reconciled-false-condition",
			monitoringStackStatusFound: obov1alpha1.MonitoringStackStatus{
				Conditions: []obov1alpha1.Condition{
					{
						Type:    obov1alpha1.ReconciledCondition,
						Status:  obov1alpha1.ConditionFalse,
						Message: "foo",
					},
				},
			},
			expectedAvailableStatusToPropagate: false,
			expectedAddonStatusMessage:         "MonitoringStack is not ready: MonitoringStack failed to reconcile: foo",
		},
		{
			name: "available-nonexistent-reconciled-unknown-condition",
			monitoringStackStatusFound: obov1alpha1.MonitoringStackStatus{
				Conditions: []obov1alpha1.Condition{
					{
						Type:    obov1alpha1.ReconciledCondition,
						Status:  obov1alpha1.ConditionUnknown,
						Message: "foo",
					},
				},
			},
			expectedAvailableStatusToPropagate: false,
			expectedAddonStatusMessage:         "MonitoringStack is not ready: MonitoringStack failed to reconcile: foo",
		},
		{
			name: "no-condition",
			monitoringStackStatusFound: obov1alpha1.MonitoringStackStatus{
				Conditions: []obov1alpha1.Condition{},
			},
			expectedAvailableStatusToPropagate: false,
			expectedAddonStatusMessage:         "MonitoringStack is not ready: MonitoringStack pending to get reconciled",
		},
	}

	c := testutil.NewClient()

	r := &monitoringStackReconciler{
		client: c,
		scheme: testutil.NewTestSchemeWithAddonsv1alpha1(),
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			addon := testutil.NewTestAddonWithMonitoringStack()
			monitoringStack := &obov1alpha1.MonitoringStack{
				Status: tc.monitoringStackStatusFound,
			}
			isMonitoringStackAvailable := r.propagateMonitoringStackStatusToAddon(monitoringStack, addon)
			require.Equal(t, tc.expectedAvailableStatusToPropagate, isMonitoringStackAvailable)

			if !isMonitoringStackAvailable {
				require.Equal(t, v1alpha1.AddonReasonUnreadyMonitoringStack, addon.Status.Conditions[0].Reason)
				require.Equal(t, metav1.ConditionFalse, addon.Status.Conditions[0].Status)
				require.Equal(t, tc.expectedAddonStatusMessage, addon.Status.Conditions[0].Message)
				require.Equal(t, v1alpha1.PhasePending, addon.Status.Phase)
			} else {
				require.Zero(t, len(addon.Status.Conditions))
			}
		})
	}
}

// TestGetWriteRelabelConfigFromAllowlist tests the getWriteRelabelConfigFromAllowlist
// function in the addon package.
func TestGetWriteRelabelConfigFromAllowlist(t *testing.T) {
	allowlist := []string{"cpu_usage", "memory_usage", "disk_space_used"}
	expectedResult := []monv1.RelabelConfig{
		{
			SourceLabels: []monv1.LabelName{"[__name__]"},
			Separator:    nil,
			TargetLabel:  "",
			Regex:        "(cpu_usage|memory_usage|disk_space_used)",
			Modulus:      0,
			Replacement:  nil,
			Action:       "keep",
		},
	}

	result := getWriteRelabelConfigFromAllowlist(allowlist)
	assert.Equal(t, expectedResult, result, "Expected result to be %v, but got %v", expectedResult, result)
}

// TestMonitoringStackReconciler_Name ensures that the Name() method
// of the monitoringStackReconciler returned the expected name defined by
// the MONITORING_STACK_RECONCILER_NAME constant.
func TestMonitoringStackReconciler_Name(t *testing.T) {
	r := &monitoringStackReconciler{}
	expectedName := MONITORING_STACK_RECONCILER_NAME

	result := r.Name()

	assert.Equal(t, expectedName, result)
}

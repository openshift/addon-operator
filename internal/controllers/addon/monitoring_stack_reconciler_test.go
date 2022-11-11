package addon

import (
	"context"
	"testing"

	obov1alpha1 "github.com/rhobs/observability-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers"
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

		err := r.ensureMonitoringStack(ctx, addon)
		require.NoError(t, err)
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

	err := r.ensureMonitoringStack(ctx, addon)
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
		Run(func(args mock.Arguments) {
			namespacedName := args.Get(1).(types.NamespacedName)
			assert.Equal(t, getMonitoringStackName(addon.Name), namespacedName.Name)
		}).
		Return(nil)
	c.On("Update", testutil.IsContext, mock.IsType(&obov1alpha1.MonitoringStack{}), mock.Anything).
		Run(func(args mock.Arguments) {
			commonConfig, stop := parseAddonInstallConfig(controllers.LoggerFromContext(ctx), addon)
			assert.False(t, stop)

			assert.Equal(t, commonConfig.Namespace, "addon-1")
		}).
		Return(nil)

	err := r.ensureMonitoringStack(ctx, addon)
	require.NoError(t, err)
	c.AssertExpectations(t)
	c.AssertNumberOfCalls(t, "Get", 1)
	c.AssertNumberOfCalls(t, "Update", 1)
}

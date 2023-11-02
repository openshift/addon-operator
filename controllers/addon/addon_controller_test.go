package addon

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	promTestUtil "github.com/prometheus/client_golang/prometheus/testutil"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"
	"github.com/openshift/addon-operator/internal/ocm"
	"github.com/openshift/addon-operator/internal/ocm/ocmtest"
	"github.com/openshift/addon-operator/internal/testutil"
)

type reconcileErrorTestCase struct {
	reconcilerErrPresent      bool
	externalAPISyncErrPresent bool
	statusUpdateErrPresent    bool
}

var (
	_                   addonReconciler = (*mockSubReconciler)(nil)
	errMockSubReconcile                 = errors.New("failed to reconcile")
)

type mockSubReconciler struct {
	returnErr bool
}

func (m *mockSubReconciler) Name() string {
	return "mock-sub-reconciler"
}

func (m *mockSubReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if m.returnErr {
		return ctrl.Result{}, errMockSubReconcile
	}
	return ctrl.Result{}, nil
}

func TestReconcileErrorHandling(t *testing.T) {
	testCases := []reconcileErrorTestCase{
		{
			reconcilerErrPresent:      false,
			externalAPISyncErrPresent: false,
			statusUpdateErrPresent:    false,
		},
		{
			reconcilerErrPresent:      false,
			externalAPISyncErrPresent: true,
			statusUpdateErrPresent:    false,
		},
		{
			reconcilerErrPresent:      false,
			externalAPISyncErrPresent: false,
			statusUpdateErrPresent:    true,
		},
		{
			reconcilerErrPresent:      false,
			externalAPISyncErrPresent: true,
			statusUpdateErrPresent:    true,
		},
		{
			reconcilerErrPresent:      true,
			externalAPISyncErrPresent: false,
			statusUpdateErrPresent:    false,
		},
		{
			reconcilerErrPresent:      true,
			externalAPISyncErrPresent: false,
			statusUpdateErrPresent:    true,
		},
		{
			reconcilerErrPresent:      true,
			externalAPISyncErrPresent: true,
			statusUpdateErrPresent:    false,
		},
		{
			reconcilerErrPresent:      true,
			externalAPISyncErrPresent: true,
			statusUpdateErrPresent:    true,
		},
	}
	for idx, testCase := range testCases {
		client := testutil.NewClient()
		ocmClient := ocmtest.NewClient()
		recorder := metrics.NewRecorder(true, fmt.Sprintf("clusterID-%v", idx))

		r := AddonReconciler{
			Client:         client,
			ocmClient:      ocmClient,
			Log:            logr.Discard(),
			Recorder:       recorder,
			subReconcilers: []addonReconciler{},
		}

		r.statusReportingEnabled = true
		// set up mock calls based on the test case.
		addon := testutil.NewTestAddonWithCatalogSourceImage()
		addon.Finalizers = append(addon.Finalizers, cacheFinalizer)

		if testCase.reconcilerErrPresent {
			r.subReconcilers = append(r.subReconcilers, &mockSubReconciler{returnErr: true})
		} else {
			r.subReconcilers = append(r.subReconcilers, &mockSubReconciler{returnErr: false})
		}

		if testCase.externalAPISyncErrPresent {
			ocmClient.On("PostAddOnStatus", mock.Anything, mock.Anything, mock.Anything).Return(ocm.AddOnStatusResponse{}, errors.New("gateway timeout"))
		} else {
			ocmClient.On("PostAddOnStatus", mock.Anything, mock.Anything, mock.Anything).Return(ocm.AddOnStatusResponse{}, nil)
		}

		if testCase.statusUpdateErrPresent {
			client.StatusMock.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("kube api server busy"))
		} else {
			client.StatusMock.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		}

		// Return the prepared addon.
		client.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			passedAddon := (args.Get(2)).(*addonsv1alpha1.Addon)
			*passedAddon = *addon
		}).Return(nil)

		// invoke Reconciler
		_, err := r.Reconcile(context.Background(), reconcile.Request{})

		expectedErrorsNum := expectedNumErrors(testCase)
		if expectedErrorsNum == 0 {
			assert.NoError(t, err)
		} else {
			multiErr, ok := err.(*multierror.Error) //nolint
			assert.True(t, ok, "expected multi error")
			assert.Equal(t, expectedNumErrors(testCase), multiErr.Len())
		}

		metric := recorder.GetReconcileErrorMetric()
		assert.NotNil(t, metric)

		if testCase.externalAPISyncErrPresent {
			// Ensure sync error during reconcile was collected as a metric
			controllerMetricVal := promTestUtil.ToFloat64(
				metric.WithLabelValues(
					"addon",
					controllers.ErrSyncWithExternalAPIs.Error(),
					addon.Name,
				),
			)
			assert.True(t, controllerMetricVal > 0)
		}
		if testCase.reconcilerErrPresent {
			// Ensure reconcile error was collected as a metric
			controllerMetricVal := promTestUtil.ToFloat64(
				metric.WithLabelValues(
					"addon",
					errMockSubReconcile.Error(),
					addon.Name,
				),
			)
			assert.True(t, controllerMetricVal > 0)
		}
	}
}

func expectedNumErrors(testCase reconcileErrorTestCase) int {
	res := 0
	if testCase.externalAPISyncErrPresent {
		res += 1
	}
	if testCase.reconcilerErrPresent {
		res += 1
	}
	if testCase.statusUpdateErrPresent {
		res += 1
	}
	return res
}

// The TestAddonReconciler_GetOCMClusterInfo function verifies that the GetOCMClusterInfo
// method of the AddonReconciler struct returns the expected OCM cluster information.
func TestAddonReconciler_GetOCMClusterInfo(t *testing.T) {
	// Create a test instance of AddonReconciler
	reconciler := &AddonReconciler{
		ocmClientMux: sync.RWMutex{},
		ocmClient:    ocmtest.NewClient(),
	}

	// Call the GetOCMClusterInfo function
	result := reconciler.GetOCMClusterInfo()

	// Set up the expected cluster info
	want := OcmClusterInfo{
		ID:   "1ou",
		Name: "openshift-mock-cluster-name",
	}

	// Assert the expected result
	assert.Equal(t, want, result, "Unexpected OCM cluster info")
}

// TestEnableGlobalPause ensures that the EnableGlobalPause method
// sets the globalPause flag to true and does not procude any errors.
func TestEnableGlobalPause(t *testing.T) {
	client := testutil.NewClient()
	ocmClient := ocmtest.NewClient()
	r := AddonReconciler{
		Client:         client,
		ocmClient:      ocmClient,
		Log:            logr.Discard(),
		subReconcilers: []addonReconciler{},
	}

	addonList := &addonsv1alpha1.AddonList{}
	client.On("List", mock.AnythingOfType("*context.emptyCtx"), addonList, mock.Anything).Return(nil).Once()

	ctx := context.Background()

	// Call the EnableGlobalPause method
	err := r.EnableGlobalPause(ctx)

	// Assert that no error occurred
	assert.NoError(t, err)

	// Assert that the globalPause flag is set to true
	assert.True(t, r.globalPause)
}

// TestDisableGlobalPause ensures that the DisableGlobalPause method
// sets the globalPause flag to false and does not product any errors.
func TestDisableGlobalPause(t *testing.T) {
	client := testutil.NewClient()
	ocmClient := ocmtest.NewClient()
	r := AddonReconciler{
		Client:         client,
		ocmClient:      ocmClient,
		Log:            logr.Discard(),
		subReconcilers: []addonReconciler{},
	}

	ctx := context.Background()

	r.globalPause = true

	addonList := &addonsv1alpha1.AddonList{}
	client.On("List", mock.AnythingOfType("*context.emptyCtx"), addonList, mock.Anything).Return(nil).Once()

	err := r.DisableGlobalPause(ctx)

	// Assert that no error occurred
	assert.NoError(t, err)

	// Assert that the globalPause flag is set to false
	assert.False(t, r.globalPause)
}

// TestInjectOCMClient ensures that the InjectOCMClient method properly injects
// the ocmClient and does not produce any errors.
func TestInjectOCMClient(t *testing.T) {
	reconcilerMock := &mockAddonReconciler{}

	client := testutil.NewClient()
	ocmClient := &ocm.Client{}
	r := &AddonReconciler{
		Client:         client,
		ocmClient:      nil,
		Log:            logr.Discard(),
		subReconcilers: []addonReconciler{},
	}

	ctx := context.Background()

	addonList := &addonsv1alpha1.AddonList{}
	client.On("List", mock.AnythingOfType("*context.emptyCtx"), addonList, mock.Anything).Return(nil).Once()

	reconcilerMock.On("requeueAllAddons", ctx).Return(nil)

	err := r.InjectOCMClient(ctx, ocmClient)

	// Assert that no error occurred
	assert.NoError(t, err)

	// Assert that the ocmClient is injected properly
	assert.Equal(t, ocmClient, r.ocmClient)
}

type mockAddonReconciler struct {
	mock.Mock
}

// TestSetupWithManager_OperatorResourceHandlerNil validates that the
// SetupWithManager method is not called with a nil operatorResourceHandler.
func TestSetupWithManager_OperatorResourceHandlerNil(t *testing.T) {
	reconciler := &AddonReconciler{
		operatorResourceHandler: nil, // Set the operatorResourceHandler to nil
	}

	err := reconciler.SetupWithManager(nil)

	expectedError := fmt.Errorf("operatorResourceHandler cannot be nil")
	assert.EqualError(t, err, expectedError.Error())
}

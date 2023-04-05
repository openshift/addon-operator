package addon

import (
	"context"
	"errors"
	"testing"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"

	"github.com/go-logr/logr"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/addon-operator/internal/ocm"
	"github.com/openshift/addon-operator/internal/ocm/ocmtest"
	"github.com/openshift/addon-operator/internal/testutil"
)

type reconcileErrorTestCase struct {
	reconcilerErrPresent      bool
	externalAPISyncErrPresent bool
	statusUpdateErrPresent    bool
}

var _ addonReconciler = (*mockSubReconciler)(nil)

type mockSubReconciler struct {
	returnErr bool
}

func (m *mockSubReconciler) Name() string {
	return "mock-sub-reconciler"
}

func (m *mockSubReconciler) Reconcile(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	if m.returnErr {
		return ctrl.Result{}, errors.New("failed to reconcile")
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
	for _, testCase := range testCases {
		client := testutil.NewClient()
		ocmClient := ocmtest.NewClient()
		r := AddonReconciler{
			Client:         client,
			ocmClient:      ocmClient,
			Log:            logr.Discard(),
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
			ocmClient.On("GetAddOnStatus", mock.Anything, mock.Anything).Return(ocm.AddOnStatusResponse{}, errors.New("gateway timeout"))
			ocmClient.On("PatchAddOnStatus", mock.Anything, mock.Anything, mock.Anything).Return(ocm.AddOnStatusResponse{}, errors.New("gateway timeout"))
		} else {
			ocmClient.On("GetAddOnStatus", mock.Anything, mock.Anything).Return(ocm.AddOnStatusResponse{}, nil)
			ocmClient.On("PatchAddOnStatus", mock.Anything, mock.Anything, mock.Anything).Return(ocm.AddOnStatusResponse{}, nil)
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

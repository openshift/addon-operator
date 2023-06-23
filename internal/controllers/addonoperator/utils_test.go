package addonoperator

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/testutil"
)

func TestAddonOperatorReconciler_handleAddonOperatorCreation(t *testing.T) {
	mockClient := &testutil.Client{}

	r := &AddonOperatorReconciler{
		Client: mockClient,
		Log:    logr.Logger{},
	}

	expectedObj := &addonsv1alpha1.AddonOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: addonsv1alpha1.DefaultAddonOperatorName,
		},
	}

	// Set the expected behavior of the mock client's Create method
	mockClient.On("Create", mock.Anything, expectedObj, mock.AnythingOfType("[]client.CreateOption")).Return(nil)

	// Call the function being tested
	err := r.handleAddonOperatorCreation(context.Background(), logr.Discard())

	// Assert that no error occurred
	assert.NoError(t, err)

	// Verify the expectations on the mock client
	mockClient.AssertExpectations(t)
}

// TestAddonOperatorReconciler_reportAddonOperatorReadinessStatus ensures that the
// reportAddonOperatorReadinessStatus method updates the status of the AddonOperator.
func TestAddonOperatorReconciler_reportAddonOperatorReadinessStatus(t *testing.T) {
	mockClient := &testutil.Client{
		StatusMock: &testutil.StatusClient{},
	}

	addonOperator := &addonsv1alpha1.AddonOperator{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{Name: addonsv1alpha1.DefaultAddonOperatorName, Namespace: "test-namespace"},
		Spec:       addonsv1alpha1.AddonOperatorSpec{},
		Status:     addonsv1alpha1.AddonOperatorStatus{},
	}

	// Set the expected behavior of the mock client's Update method
	mockClient.StatusMock.On("Update", mock.Anything, addonOperator, mock.AnythingOfType("[]client.SubResourceUpdateOption")).Return(nil)

	r := &AddonOperatorReconciler{
		Client: mockClient,
	}

	err := r.reportAddonOperatorReadinessStatus(context.Background(), addonOperator)
	assert.NoError(t, err)

	// Verify the expectations on the mock client
	mockClient.StatusMock.AssertExpectations(t)

	// Assert the updated values in the AddonOperator object
	assert.Equal(t, metav1.ConditionTrue, addonOperator.Status.Conditions[0].Status)
	assert.Equal(t, addonsv1alpha1.AddonOperatorReasonReady, addonOperator.Status.Conditions[0].Reason)
	assert.Equal(t, addonsv1alpha1.PhaseReady, addonOperator.Status.Phase)
	assert.Equal(t, addonOperator.Generation, addonOperator.Status.ObservedGeneration)
	assert.NotNil(t, addonOperator.Status.LastHeartbeatTime)
}

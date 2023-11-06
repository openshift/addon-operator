package ocm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
)

// TestClient_GetAddOnStatus verifies the behavior of the GetAddOnStatus method using
// different scenarios.
func TestClient_GetAddOnStatus(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock the response
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{
			"kind": "AddOnStatusResponse",
			"addon_id": "addon-1",
			"correlation_id": "12345-6789",
			"version": "2.0.13",
			"status_conditions": []
		}`))
		require.NoError(t, err, "Error writing response")
	}))
	defer server.Close()

	// Create a new client with the mock server's URL
	client := &Client{
		opts: ClientOptions{
			Endpoint:  server.URL,
			ClusterID: "1ou",
		},
		httpClient: server.Client(),
	}

	res, err := client.GetAddOnStatus(context.Background(), "addon-1")

	// Check the result
	require.NoError(t, err, "Client.GetAddOnStatus() returned an error")

	expected := AddOnStatusResponse{
		Kind:             "AddOnStatusResponse",
		AddonID:          "addon-1",
		CorrelationID:    "12345-6789",
		AddonVersion:     "2.0.13",
		StatusConditions: []addonsv1alpha1.AddOnStatusCondition{},
	}

	require.Equal(t, expected, res, "Client.GetAddOnStatus() returned unexpected result")

	// Test the case where the server returns an error response
	serverError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock the error response
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(`Internal Server Error`))
		require.NoError(t, err, "Error writing response")
	}))
	defer serverError.Close()

	clientError := &Client{
		opts: ClientOptions{
			Endpoint:  serverError.URL,
			ClusterID: "1ou",
		},
		httpClient: serverError.Client(),
	}

	// Call the function to test
	_, err = clientError.GetAddOnStatus(context.Background(), "addon-1")

	// Check the error
	require.Error(t, err, "Client.GetAddOnStatus() should return an error for server error")
}

// TestClient_PostAddOnStatus verifies the behavior of the PatchAddOnStatus method using
// different scenarios.
func TestClient_PostAddOnStatus(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock the response
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{
			"kind": "AddOnStatusResponse",
			"addon_id": "addon-1",
			"correlation_id": "12345-6789",
			"version": "2.0.13",
			"status_conditions": []
		}`))
		require.NoError(t, err, "Error writing response")
	}))
	defer server.Close()

	// Create a new client with the mock server's URL
	client := &Client{
		opts: ClientOptions{
			Endpoint:  server.URL,
			ClusterID: "your-cluster-id",
		},
		httpClient: http.DefaultClient,
	}

	// Prepare the payload for the PostAddOnStatus method
	payload := AddOnStatusPostRequest{
		AddonID:          "addon-1",
		CorrelationID:    "12345-6789",
		AddonVersion:     "2.0.13",
		StatusConditions: []addonsv1alpha1.AddOnStatusCondition{},
	}

	res, err := client.PostAddOnStatus(context.Background(), payload)

	// Check the result
	if err != nil {
		t.Errorf("Client.PostAddOnStatus() error = %v", err)
		return
	}

	expected := AddOnStatusResponse{
		Kind:             "AddOnStatusResponse",
		AddonID:          "addon-1",
		CorrelationID:    "12345-6789",
		AddonVersion:     "2.0.13",
		StatusConditions: []addonsv1alpha1.AddOnStatusCondition{},
	}

	require.Equal(t, expected, res, "Client.PostAddOnStatus() returned unexpected result")

	// Test the case where the server returns an error response
	serverError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock the error response
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(`Internal Server Error`))
		require.NoError(t, err, "Error writing response")
	}))
	defer serverError.Close()

	clientError := &Client{
		opts: ClientOptions{
			Endpoint:  serverError.URL,
			ClusterID: "your-cluster-id",
		},
		httpClient: http.DefaultClient,
	}

	// Call the function to test
	_, err = clientError.PostAddOnStatus(context.Background(), payload)

	// Check the error
	require.Error(t, err, "Client.PostAddOnStatus() should return an error for server error")
}

// TestClient_PatchAddOnStatus verifies the behavior of the PatchAddOnStatus using different
// scenarios.
func TestClient_PatchAddOnStatus(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock the response
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{
			"kind": "AddOnStatusResponse",
			"addon_id": "addon-1",
			"correlation_id": "12345-6789",
			"version": "2.0.13",
			"status_conditions": [
				{
					"status_type": "Ready",
					"status_value": "True",
					"reason": "InstallationSuccessful",
					"message": "InstallationSuccessful"
				}
			]
		}`))
		require.NoError(t, err, "Error writing response")
	}))
	defer server.Close()

	// Create a new client with the mock server's URL
	client := &Client{
		opts: ClientOptions{
			Endpoint:  server.URL,
			ClusterID: "1ou",
		},
		httpClient: http.DefaultClient,
	}

	// Prepare the payload
	payload := AddOnStatusPatchRequest{
		CorrelationID: "12345-6789",
		AddonVersion:  "2.0.13",
		StatusConditions: []addonsv1alpha1.AddOnStatusCondition{
			{
				StatusType:  "Ready",
				StatusValue: metav1.ConditionTrue,
				Reason:      "InstallationSuccessful",
				Message:     "InstallationSuccessful",
			},
		},
	}

	res, err := client.PatchAddOnStatus(context.Background(), "addon-1", payload)

	// Check the error
	require.NoError(t, err, "Client.PatchAddOnStatus() returned an error")

	// Check the result
	expected := AddOnStatusResponse{
		Kind:          "AddOnStatusResponse",
		AddonID:       "addon-1",
		CorrelationID: "12345-6789",
		AddonVersion:  "2.0.13",
		StatusConditions: []addonsv1alpha1.AddOnStatusCondition{
			{
				StatusType:  "Ready",
				StatusValue: metav1.ConditionTrue,
				Reason:      "InstallationSuccessful",
				Message:     "InstallationSuccessful",
			},
		},
	}
	require.Equal(t, expected, res, "Client.PatchAddOnStatus() returned unexpected result")

	// Test the case where the server returns an error response
	serverError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock the error response
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(`Internal Server Error`))
		require.NoError(t, err, "Error writing response")
	}))
	defer serverError.Close()

	clientError := &Client{
		opts: ClientOptions{
			Endpoint:  serverError.URL,
			ClusterID: "1ou",
		},
		httpClient: http.DefaultClient,
	}

	_, err = clientError.PatchAddOnStatus(context.Background(), "addon-1", payload)

	// Check the error
	require.Error(t, err, "Client.PatchAddOnStatus() should return an error for server error")
}

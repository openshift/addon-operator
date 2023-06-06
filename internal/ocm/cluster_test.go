package ocm

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"net/http"
)

func TestClient_GetClusterIDAndName(t *testing.T) {
	// Create a Client instance with the desired options
	c := &Client{
		opts:       ClientOptions{ClusterID: "123456-7890-abc1-0987624c", ClusterName: "my-test-cluster"},
		httpClient: &http.Client{},
	}

	// Call the GetClusterIDAndName function
	clusterID, clusterName := c.GetClusterIDAndName()

	// Assert the expected cluster ID and cluster name
	expectedClusterID := "123456-7890-abc1-0987624c"
	expectedClusterName := "my-test-cluster"
	assert.Equal(t, expectedClusterID, clusterID, "Cluster ID doesn't match")
	assert.Equal(t, expectedClusterName, clusterName, "Cluster name doesn't match")
}
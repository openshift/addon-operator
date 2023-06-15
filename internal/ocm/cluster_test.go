package ocm

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The TestClient_GetCLusterIDAndName function retrieves the cluster ID and cluster name stored
// within a Client object.
func TestClient_GetClusterIDAndName(t *testing.T) {
	tests := []struct {
		name                string
		opts                ClientOptions
		expectedClusterID   string
		expectedClusterName string
	}{
		{
			name: "Default Options",
			opts: ClientOptions{
				ClusterID:   "123456-7890-abc1-0987624c",
				ClusterName: "my-test-cluster",
			},
			expectedClusterID:   "123456-7890-abc1-0987624c",
			expectedClusterName: "my-test-cluster",
		},
		{
			name: "Empty Options",
			opts: ClientOptions{
				ClusterID:   "",
				ClusterName: "",
			},
			expectedClusterID:   "",
			expectedClusterName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{
				opts:       tt.opts,
				httpClient: &http.Client{},
			}

			clusterID, clusterName := c.GetClusterIDAndName()

			assert.Equal(t, tt.expectedClusterID, clusterID, "Cluster ID does not match")
			assert.Equal(t, tt.expectedClusterName, clusterName, "Cluster name does not match")
		})
	}
}

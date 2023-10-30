package ocmtest

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/openshift/addon-operator/internal/ocm"
)

const (
	MockClusterId   = "1ou"
	MockClusterName = "openshift-mock-cluster-name"
)

type Client struct {
	mock.Mock
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) PatchUpgradePolicy(
	ctx context.Context,
	req ocm.UpgradePolicyPatchRequest,
) (ocm.UpgradePolicyPatchResponse, error) {
	args := c.Called(ctx, req)
	return args.Get(0).(ocm.UpgradePolicyPatchResponse),
		args.Error(1)
}

func (c *Client) GetUpgradePolicy(
	ctx context.Context,
	req ocm.UpgradePolicyGetRequest,
) (ocm.UpgradePolicyGetResponse, error) {
	args := c.Called(ctx, req)
	return args.Get(0).(ocm.UpgradePolicyGetResponse),
		args.Error(1)
}

func (c *Client) GetCluster(
	ctx context.Context,
	req ocm.ClusterGetRequest,
) (ocm.ClusterGetResponse, error) {
	args := c.Called(ctx, req)
	return args.Get(0).(ocm.ClusterGetResponse),
		args.Error(1)
}

func (c *Client) GetClusterIDAndName() (string, string) {
	return MockClusterId, MockClusterName
}

func (c *Client) PostAddOnStatus(
	ctx context.Context,
	req ocm.AddOnStatusPostRequest,
) (ocm.AddOnStatusResponse, error) {
	args := c.Called(ctx, req)
	return args.Get(0).(ocm.AddOnStatusResponse),
		args.Error(1)
}

func (c *Client) PatchAddOnStatus(
	ctx context.Context,
	addonID string,
	req ocm.AddOnStatusPatchRequest,
) (ocm.AddOnStatusResponse, error) {
	args := c.Called(ctx, addonID, req)
	return args.Get(0).(ocm.AddOnStatusResponse),
		args.Error(1)
}

func (c *Client) GetAddOnStatus(
	ctx context.Context,
	addonID string,
) (ocm.AddOnStatusResponse, error) {
	args := c.Called(ctx, addonID)
	return args.Get(0).(ocm.AddOnStatusResponse),
		args.Error(1)
}

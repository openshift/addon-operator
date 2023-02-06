package ocm

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	addonsv1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
)

type AddOnStatusPostRequest struct {
	AddonID string `json:"addon_id"`
	// Correlation ID for co-relating current AddonCR revision and reported status.
	CorrelationID string `json:"correlation_id"`
	// Reported addon status conditions
	StatusConditions []addonsv1alpha1.AddOnStatusCondition `json:"status_conditions"`
}

type AddOnStatusPatchRequest struct {
	// Correlation ID for co-relating current AddonCR revision and reported status.
	CorrelationID string `json:"correlation_id"`
	// Reported addon status conditions
	StatusConditions []addonsv1alpha1.AddOnStatusCondition `json:"status_conditions"`
}

type AddOnStatusGetRequest struct{}

type AddOnStatusResponse struct {
	Kind    string `json:"kind"`
	AddonID string `json:"addon_id"`
	// Correlation ID for co-relating current AddonCR revision and reported status.
	CorrelationID string `json:"correlation_id"`
	// Reported addon status conditions
	StatusConditions []addonsv1alpha1.AddOnStatusCondition `json:"status_conditions"`
}

func (c *Client) GetAddOnStatus(ctx context.Context, addonID string) (AddOnStatusResponse, error) {
	res := &AddOnStatusResponse{}
	err := c.do(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/api/addons_mgmt/v1/clusters/%s/status/%s", c.opts.ClusterID, addonID),
		url.Values{},
		AddOnStatusGetRequest{},
		res,
	)
	if err != nil {
		return AddOnStatusResponse{}, err
	}
	return *res, nil
}

func (c *Client) PostAddOnStatus(ctx context.Context, payload AddOnStatusPostRequest) (AddOnStatusResponse, error) {
	res := &AddOnStatusResponse{}
	err := c.do(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/api/addons_mgmt/v1/clusters/%s/status", c.opts.ClusterID),
		url.Values{},
		payload,
		res,
	)
	if err != nil {
		return AddOnStatusResponse{}, err
	}
	return *res, nil
}

func (c *Client) PatchAddOnStatus(ctx context.Context, addonID string, payload AddOnStatusPatchRequest) (AddOnStatusResponse, error) {
	res := &AddOnStatusResponse{}
	err := c.do(
		ctx,
		http.MethodPatch,
		fmt.Sprintf("/api/addons_mgmt/v1/clusters/%s/status/%s", c.opts.ClusterID, addonID),
		url.Values{},
		payload,
		res,
	)
	if err != nil {
		return AddOnStatusResponse{}, err
	}
	return *res, nil
}

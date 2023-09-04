package testutil

import (
	"context"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client is a mock for the controller-runtime dynamic client interface.
type Client struct {
	mock.Mock

	StatusMock *StatusClient
}

var _ client.Client = &Client{}

func NewClient() *Client {
	return &Client{
		StatusMock: &StatusClient{},
	}
}

func (c *Client) SubResource(name string) client.SubResourceClient {
	c.Called(name)
	return nil
}

// StatusClient interface

func (c *Client) Status() client.StatusWriter {
	return c.StatusMock
}

// Reader interface

func (c *Client) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	args := c.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (c *Client) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := c.Called(ctx, list, opts)
	return args.Error(0)
}

// Writer interface

func (c *Client) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	args := c.Called(ctx, obj, opts)
	return args.Error(0)
}

func (c *Client) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	args := c.Called(ctx, obj, opts)
	return args.Error(0)
}

func (c *Client) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	args := c.Called(ctx, obj, opts)
	return args.Error(0)
}

func (c *Client) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	args := c.Called(ctx, obj, patch, opts)
	return args.Error(0)
}

func (c *Client) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	args := c.Called(ctx, obj, opts)
	return args.Error(0)
}

func (c *Client) Scheme() *runtime.Scheme {
	args := c.Called()
	return args.Get(0).(*runtime.Scheme)
}

func (c *Client) RESTMapper() meta.RESTMapper {
	args := c.Called()
	return args.Get(0).(meta.RESTMapper)
}

type StatusClient struct {
	mock.Mock
}

var _ client.StatusWriter = &StatusClient{}

func (c *StatusClient) Update(
	ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	args := c.Called(ctx, obj, opts)
	return args.Error(0)
}

func (c *StatusClient) Patch(
	ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	args := c.Called(ctx, obj, patch, opts)
	return args.Error(0)
}

func (c *StatusClient) Create(ctx context.Context, obj client.Object, obj2 client.Object, opts ...client.SubResourceCreateOption) error {
	args := c.Called(ctx, obj, obj2, opts)
	return args.Error(0)
}

// GroupVersionKindFor implements client.Client.
func (*Client) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{
		Group:   "group",
		Version: "version",
		Kind:    "kind",
	}, nil
}

// IsObjectNamespaced implements client.Client.
func (*Client) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

package runtimeoptionstest

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/openshift/addon-operator/internal/controllers/runtimeoptions"
)

type RuntimeOptionMock struct {
	mock.Mock
}

var _ runtimeoptions.Option = (*RuntimeOptionMock)(nil)

func (r *RuntimeOptionMock) Enable(ctx context.Context) error {
	args := r.Called(ctx)
	return args.Error(0)
}

func (r *RuntimeOptionMock) Disable(ctx context.Context) error {
	args := r.Called(ctx)
	return args.Error(0)
}

func (r *RuntimeOptionMock) Name() string {
	args := r.Called()
	return args.Get(0).(string)
}

func (r *RuntimeOptionMock) Enabled() bool {
	args := r.Called()
	return args.Get(0).(bool)
}

func (r *RuntimeOptionMock) SetControllerActionOnDisable(action func(context.Context) error) {
	r.Called(action)
}

func (r *RuntimeOptionMock) SetControllerActionOnEnable(action func(context.Context) error) {
	r.Called(action)
}

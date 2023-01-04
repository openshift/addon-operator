package runtimeoptions

import (
	"sync"

	"golang.org/x/net/context"
)

type Option interface {
	OptionManager
	OptionConsumer
}

// OptionManager interface is supposed to be used by the controller/s
// which wishes to manage the concerned runtime option. IE, enable
// or disable an option.
type OptionManager interface {
	Name() string
	Enable(ctx context.Context) error
	Disable(ctx context.Context) error
}

// OptionConsumer interface is supposed to be used by the controller
// which wishes to consume the current option state. This controller
// can also register an action which needs to be run on option value
// transitions.
type OptionConsumer interface {
	Enabled() bool
	Name() string
	SetControllerActionOnEnable(func(context.Context) error)
	SetControllerActionOnDisable(func(context.Context) error)
}

type option struct {
	name    string
	enabled bool
	sync.RWMutex
	controllerActionOnTrue  func(context.Context) error
	controllerActionOnFalse func(context.Context) error
}

type controllerActionOnValue func(*option)

func WithControllerActionOnEnable(action func(context.Context) error) controllerActionOnValue {
	return func(o *option) {
		o.SetControllerActionOnEnable(action)
	}
}

func WithControllerActionOnDisable(action func(context.Context) error) controllerActionOnValue {
	return func(o *option) {
		o.SetControllerActionOnEnable(action)
	}
}

func newRuntimeOption(name string, controllerActionOpts ...controllerActionOnValue) Option {
	o := &option{
		name: name,
	}
	for _, opt := range controllerActionOpts {
		opt(o)
	}
	return o
}

func (o *option) SetControllerActionOnEnable(action func(context.Context) error) {
	o.controllerActionOnTrue = action
}

func (o *option) SetControllerActionOnDisable(action func(context.Context) error) {
	o.controllerActionOnFalse = action
}

func (o *option) Name() string {
	return o.name
}

func (o *option) Enabled() bool {
	o.RLock()
	defer o.RUnlock()
	return o.enabled
}

func (o *option) Enable(ctx context.Context) error {
	o.Lock()
	defer o.Unlock()
	o.enabled = true
	if o.controllerActionOnTrue != nil {
		return o.controllerActionOnTrue(ctx)
	}
	return nil
}

func (o *option) Disable(ctx context.Context) error {
	o.Lock()
	defer o.Unlock()
	o.enabled = false
	if o.controllerActionOnFalse != nil {
		return o.controllerActionOnFalse(ctx)
	}
	return nil
}

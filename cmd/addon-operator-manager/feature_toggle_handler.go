package main

import (
	"context"
)

type FeatureToggleHandler interface {
	Name() string
	IsEnabled() bool
	Handle(ctx context.Context) error
}

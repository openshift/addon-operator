package controllers

import (
	"context"

	"github.com/go-logr/logr"
)

type controllerContextKey int

const loggerContextKey controllerContextKey = iota

// Add a logger to the given context, to make it easier passing it around.
func ContextWithLogger(parent context.Context, logger logr.Logger) context.Context {
	return context.WithValue(context.Background(), loggerContextKey, logger)
}

// Get the logger from the given context.
// Returning a discarding logger, if none was set.
func LoggerFromContext(ctx context.Context) logr.Logger {
	logger, ok := ctx.Value(loggerContextKey).(logr.Logger)
	if ok {
		return logger
	}
	return logr.Discard()
}

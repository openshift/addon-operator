package controllers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

// The TestContextWithLogger function tests the behavior of the ContextWithLogger
// function. The purpose of the function is to create a new context with a logger
// value attached.
func TestContextWithLogger(t *testing.T) {
	tests := []struct {
		name        string
		parentCtx   context.Context
		logger      logr.Logger
		expectedCtx context.Context
	}{
		{
			name:        "Multiple Loggers",
			parentCtx:   context.Background(),
			logger:      logr.Discard(),
			expectedCtx: context.WithValue(context.Background(), loggerContextKey, logr.Discard()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ContextWithLogger(tt.parentCtx, tt.logger)
			assert.Equal(t, tt.expectedCtx, ctx, "Returned context should match the expected context")
			assert.Equal(t, tt.logger, ctx.Value(loggerContextKey), "Logger value should be added to the context")
		})
	}
}

// The TestLoggerFromContext function ensures that the LoggerFromContext
// function behaves correctly by returning the expected logger from
// the provided context, or the default logger if no logger is present.
func TestLoggerFromContext(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name string
		args args
		want logr.Logger
	}{
		{
			name: "Context with logger",
			args: args{
				ctx: ContextWithLogger(context.Background(), logr.Discard()),
			},
			want: logr.Discard(),
		},
		{
			name: "Context without logger",
			args: args{
				ctx: context.Background(),
			},
			want: logr.Discard(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LoggerFromContext(tt.args.ctx)
			assert.Equal(t, tt.want, got, "LoggerFromContext() returned unexpected logger")
		})
	}
}

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
	type args struct {
		parent context.Context
		logger logr.Logger
	}
	tests := []struct {
		name string
		args args
		want context.Context
	}{
		{
			name: "Valid logger",
			args: args{
				parent: context.Background(),
				logger: logr.Discard(),
			},
			want: context.WithValue(context.Background(), loggerContextKey, logr.Discard()),
		},
		{
			name: "Nil logger",
			args: args{
				parent: context.Background(),
				logger: logr.Discard(),
			},
			want: context.WithValue(context.Background(), loggerContextKey, logr.Discard()),
		},
		{
			name: "Existing logger in parent context",
			args: args{
				parent: ContextWithLogger(context.Background(), logr.Discard()),
				logger: logr.Discard(),
			},
			want: context.WithValue(context.Background(), loggerContextKey, logr.Discard()),
		},
		{
			name: "Different key value in parent context",
			args: args{
				parent: context.WithValue(context.Background(), "otherKey", "otherValue"),
				logger: logr.Discard(),
			},
			want: context.WithValue(context.WithValue(context.Background(), "otherKey", "otherValue"), loggerContextKey, logr.Discard()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContextWithLogger(tt.args.parent, tt.args.logger)
			wantValue := tt.want.Value(loggerContextKey)
			gotValue := got.Value(loggerContextKey)

			assert.Equal(t, wantValue, gotValue, "ContextWithLogger() = %v, want %v", got, tt.want)
		})
	}
}
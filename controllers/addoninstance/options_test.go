package addoninstance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-logr/logr"
)

// TestConfigurators function tests the configuration functions
// for the addoninstance options.
func TestConfigurators(t *testing.T) {
	discardLogger := logr.Discard()

	type argsController struct {
		c *ControllerConfig
	}
	type argsPhaseCheck struct {
		c *PhaseCheckHeartbeatConfig
	}
	tests := []struct {
		name         string
		configurator interface{}
		args         interface{}
		want         interface{}
	}{
		{
			name:         "WithLog.ConfigureController sets the correct logger",
			configurator: WithLog{Log: discardLogger},
			args: argsController{c: &ControllerConfig{
				Log:             logr.Discard(),
				PollingInterval: 1 * time.Minute,
				SerialPhases:    []Phase{},
			}},
			want: discardLogger,
		},
		{
			name:         "WithLog.ConfigurePhaseCheckHeartbeat sets the correct logger",
			configurator: WithLog{Log: discardLogger},
			args: argsPhaseCheck{c: &PhaseCheckHeartbeatConfig{
				Clock:               nil,
				ThresholdMultiplier: 1,
				Log:                 logr.Discard(),
			}},
			want: discardLogger,
		},
		{
			name:         "WithPollingInterval.ConfigureController sets the correct polling interval",
			configurator: WithPollingInterval(time.Second),
			args: argsController{c: &ControllerConfig{
				PollingInterval: 0,
				SerialPhases:    nil,
				Log:             logr.Discard(),
			}},
			want: time.Second,
		},
		{
			name:         "WithThresholdMultiplier.ConfigurePhaseCheckHeartbeat sets the correct threshold multiplier",
			configurator: WithThresholdMultiplier(2),
			args: argsPhaseCheck{c: &PhaseCheckHeartbeatConfig{
				Clock:               nil,
				ThresholdMultiplier: 0,
			}},
			want: int64(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch c := tt.configurator.(type) {
			case WithLog:
				if args, ok := tt.args.(argsController); ok {
					c.ConfigureController(args.c)
					assert.Equal(t, tt.want, args.c.Log)
				} else if args, ok := tt.args.(argsPhaseCheck); ok {
					c.ConfigurePhaseCheckHeartbeat(args.c)
					assert.Equal(t, tt.want, args.c.Log)
				}
			case WithPollingInterval:
				if args, ok := tt.args.(argsController); ok {
					c.ConfigureController(args.c)
					assert.Equal(t, tt.want, args.c.PollingInterval)
				}
			case WithThresholdMultiplier:
				if args, ok := tt.args.(argsPhaseCheck); ok {
					c.ConfigurePhaseCheckHeartbeat(args.c)
					assert.Equal(t, tt.want, args.c.ThresholdMultiplier)
				}
			default:
				require.Fail(t, "unknown configurator type")
			}
		})
	}
}

package addoninstance

import (
	"time"

	"github.com/go-logr/logr"
)

type WithClock struct{ Clock Clock }

func (w WithClock) ConfigurePhaseCheckHeartbeat(c *PhaseCheckHeartbeatConfig) {
	c.Clock = w.Clock
}

type WithLog struct{ Log logr.Logger }

func (w WithLog) ConfigureController(c *ControllerConfig) {
	c.Log = w.Log
}

func (w WithLog) ConfigurePhaseCheckHeartbeat(c *PhaseCheckHeartbeatConfig) {
	c.Log = w.Log
}

type WithPollingInterval time.Duration

func (w WithPollingInterval) ConfigureController(c *ControllerConfig) {
	c.PollingInterval = time.Duration(w)
}

type WithSerialPhases []Phase

func (w WithSerialPhases) ConfigureController(c *ControllerConfig) {
	c.SerialPhases = []Phase(w)
}

type WithThresholdMultiplier int64

func (w WithThresholdMultiplier) ConfigurePhaseCheckHeartbeat(c *PhaseCheckHeartbeatConfig) {
	c.ThresholdMultiplier = int64(w)
}

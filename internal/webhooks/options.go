package webhooks

import (
	"github.com/go-logr/logr"
)

type WithLogger struct{ Log logr.Logger }

func (w WithLogger) ConfigureDefaultAddonValidator(c *DefaultAddonValidatorConfig) {
	c.Log = w.Log
}

package addoninstance

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	av1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers/addoninstance/internal/phase"
)

const (
	conditionHealthyMessageWaiting              = "Waiting for first heartbeat."
	conditionHealthyMessageHeartbeatReceived    = "Heartbeat received within threshold."
	conditionHealthyMessageHeartbeatNotReceived = "Heartbeat not received before timeout threshold."
)

func NewPhaseCheckHeartbeat(opts ...PhaseCheckHeartbeatOption) *PhaseCheckHeartbeat {
	var cfg PhaseCheckHeartbeatConfig

	cfg.Option(opts...)
	cfg.Default()

	return &PhaseCheckHeartbeat{
		cfg: cfg,
	}
}

type PhaseCheckHeartbeat struct {
	cfg PhaseCheckHeartbeatConfig
}

func (c *PhaseCheckHeartbeat) Execute(ctx context.Context, req phase.Request) phase.Result {
	instance := req.Instance

	log := c.cfg.Log.WithValues(
		"namespace", instance.Namespace,
		"name", instance.Name,
	)

	lastHeartbeatTime := instance.Status.LastHeartbeatTime
	if lastHeartbeatTime.IsZero() {
		log.Info("waiting for first heartbeat")

		return phase.Success(metav1.Condition{
			Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
			Status:  "Unknown",
			Reason:  av1alpha1.AddonInstanceHealthyReasonPendingFirstHeartbeat.String(),
			Message: conditionHealthyMessageWaiting,
		})
	}

	threshold := time.Duration(c.cfg.ThresholdMultiplier) * instance.Spec.HeartbeatUpdatePeriod.Duration
	if c.cfg.Clock.Now().After(lastHeartbeatTime.Add(threshold)) {
		log.Info("heartbeat not received by timeout threshold")

		return phase.Success(metav1.Condition{
			Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
			Status:  "Unknown",
			Reason:  av1alpha1.AddonInstanceHealthyReasonHeartbeatTimeout.String(),
			Message: conditionHealthyMessageHeartbeatNotReceived,
		})
	}

	log.Info("receiving heartbeats")

	return phase.Success(metav1.Condition{
		Type:    av1alpha1.AddonInstanceConditionHealthy.String(),
		Status:  "True",
		Reason:  av1alpha1.AddonInstanceHealthyReasonReceivingHeartbeats.String(),
		Message: conditionHealthyMessageHeartbeatReceived,
	})
}

func (p *PhaseCheckHeartbeat) String() string {
	return "PhaseCheckHeartbeat"
}

type PhaseCheckHeartbeatConfig struct {
	Log                 logr.Logger
	Clock               Clock
	ThresholdMultiplier int64
}

func (c *PhaseCheckHeartbeatConfig) Option(opts ...PhaseCheckHeartbeatOption) {
	for _, opt := range opts {
		opt.ConfigurePhaseCheckHeartbeat(c)
	}
}

func (c *PhaseCheckHeartbeatConfig) Default() {
	if c.Log.GetSink() == nil {
		c.Log = logr.Discard()
	}

	if c.Clock == nil {
		c.Clock = NewDefaultClock()
	}

	if c.ThresholdMultiplier == 0 {
		c.ThresholdMultiplier = 3
	}
}

type PhaseCheckHeartbeatOption interface {
	ConfigurePhaseCheckHeartbeat(*PhaseCheckHeartbeatConfig)
}

type Clock interface {
	Now() time.Time
}

func NewDefaultClock() DefaultClock {
	return DefaultClock{}
}

type DefaultClock struct{}

func (c DefaultClock) Now() time.Time {
	return metav1.Now().Time
}

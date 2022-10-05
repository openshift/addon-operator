package addoninstance

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	av1alpha1 "github.com/openshift/addon-operator/apis/addons/v1alpha1"
	"github.com/openshift/addon-operator/internal/controllers/addoninstance/internal/phase"
)

func NewController(c client.Client, opts ...ControllerOption) *Controller {
	var cfg ControllerConfig

	cfg.Option(opts...)
	cfg.Default()

	return &Controller{
		cfg:    cfg,
		client: c,
	}
}

type Controller struct {
	cfg    ControllerConfig
	client client.Client
}

func (r *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&av1alpha1.AddonInstance{}).
		Complete(r)
}

func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := c.cfg.Log.WithValues(
		"namespace", req.Namespace,
		"name", req.Name,
	)

	var instance av1alpha1.AddonInstance
	if err := c.client.Get(ctx, req.NamespacedName, &instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("reconciling AddonInstance")

	var conditions []metav1.Condition

	for _, p := range c.cfg.SerialPhases {
		res := p.Execute(ctx, phase.Request{Instance: instance})
		if err := res.Error(); err != nil {
			log.Error(err, "reconciliation failed")

			return ctrl.Result{}, fmt.Errorf("executing phase %q: %w", p, err)
		}

		conditions = append(conditions, res.Conditions...)
	}

	for _, cond := range conditions {
		apimeta.SetStatusCondition(&instance.Status.Conditions, cond)
	}

	instance.Status.ObservedGeneration = instance.Generation

	log.Info("updating status conditions")

	if err := c.client.Status().Update(ctx, &instance); err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"updating status for AddonInstance '%s/%s': %w", instance.Namespace, instance.Name, err,
		)
	}

	log.Info("successfully reconciled AddonInstance")

	return ctrl.Result{RequeueAfter: c.cfg.PollingInterval}, nil
}

type ControllerConfig struct {
	Log                 logr.Logger
	PollingInterval     time.Duration
	SerialPhases        []Phase
}

func (c *ControllerConfig) Option(opts ...ControllerOption) {
	for _, opt := range opts {
		opt.ConfigureController(c)
	}
}

func (c *ControllerConfig) Default() {
	if c.Log.GetSink() == nil {
		c.Log = logr.Discard()
	}

	if c.PollingInterval == 0 {
		c.PollingInterval = 10 * time.Second
	}
}

type ControllerOption interface {
	ConfigureController(c *ControllerConfig)
}

type Phase interface {
	Execute(ctx context.Context, req phase.Request) phase.Result
	fmt.Stringer
}

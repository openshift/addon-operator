package addoninstance

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	av1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/controllers/addoninstance/internal/phase"
	"github.com/openshift/addon-operator/internal/metrics"
)

func NewController(c client.Client, opts ...ControllerOption) *Controller {
	var cfg ControllerConfig

	cfg.Option(opts...)
	cfg.Default()

	return &Controller{
		cfg:    cfg,
		client: NewAddonInstanceClient(c),
	}
}

type Controller struct {
	cfg    ControllerConfig
	client AddonInstanceClient
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
	reconErr := metrics.NewReconcileError("addoninstance", c.cfg.Recorder, true)
	instance, err := c.client.Get(ctx, req.Name, req.Namespace)
	if err != nil {
		reconErr.Report(controllers.ErrGetAddonInstance, req.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	defer func() {
		log.Info("updating status conditions")

		if err := c.client.UpdateStatus(ctx, instance); err != nil {
			reconErr.Report(controllers.ErrUpdateAddonInstanceStatus, req.Name)
			log.Error(err, "updating AddonInstance status")
		}
	}()

	log.Info("reconciling AddonInstance")

	for _, p := range c.cfg.SerialPhases {
		res := p.Execute(ctx, phase.Request{Instance: *instance})

		for _, cond := range res.Conditions {
			cond.ObservedGeneration = instance.Generation

			apimeta.SetStatusCondition(&instance.Status.Conditions, cond)
		}

		if err := res.Error(); err != nil {
			reconErr.Report(controllers.ErrExecuteAddonInstanceReconcilePhase, req.Name)
			return ctrl.Result{}, fmt.Errorf("executing phase %q: %w", p, err)
		}
	}

	log.Info("successfully reconciled AddonInstance")

	return ctrl.Result{RequeueAfter: c.cfg.PollingInterval}, nil
}

type ControllerConfig struct {
	Log             logr.Logger
	PollingInterval time.Duration
	SerialPhases    []Phase
	Recorder        *metrics.Recorder
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

type AddonInstanceClient interface {
	Get(ctx context.Context, name, namespace string) (*av1alpha1.AddonInstance, error)
	UpdateStatus(ctx context.Context, instance *av1alpha1.AddonInstance) error
}

func NewAddonInstanceClient(client client.Client) *AddonInstanceClientImpl {
	return &AddonInstanceClientImpl{
		client: client,
	}
}

type AddonInstanceClientImpl struct {
	client client.Client
}

func (c *AddonInstanceClientImpl) Get(ctx context.Context, name, namespace string) (*av1alpha1.AddonInstance, error) {
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}

	instance := &av1alpha1.AddonInstance{}
	if err := c.client.Get(ctx, key, instance); err != nil {
		return nil, fmt.Errorf("getting AddonInstance '%s/%s': %w", namespace, name, err)
	}

	return instance, nil
}

func (c *AddonInstanceClientImpl) UpdateStatus(ctx context.Context, instance *av1alpha1.AddonInstance) error {
	instance.Status.ObservedGeneration = instance.Generation

	if err := c.client.Status().Update(ctx, instance); err != nil {
		return fmt.Errorf("updating AddonInstance status: %w", err)
	}

	return nil
}

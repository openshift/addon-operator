package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/openshift/addon-operator/controllers"
	"github.com/openshift/addon-operator/internal/metrics"

	configv1 "github.com/openshift/api/config/v1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"

	addoncontroller "github.com/openshift/addon-operator/controllers/addon"
	aictrl "github.com/openshift/addon-operator/controllers/addoninstance"
	aocontroller "github.com/openshift/addon-operator/controllers/addonoperator"
	"github.com/openshift/addon-operator/internal/featuretoggle"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(operatorsv1.AddToScheme(scheme))
	utilruntime.Must(operatorsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))

	utilruntime.Must(addonsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	log.SetLogger(zap.New(zap.UseDevMode(true)))
}

func initReconcilers(mgr ctrl.Manager,
	namespace string,
	enableRecorder bool,
	addonOperatorInCluster addonsv1alpha1.AddonOperator,
	enableStatusReporting bool,
	opts ...addoncontroller.AddonReconcilerOptions) error {
	ctx := context.Background()

	// Create a client that does not cache resources cluster-wide.
	uncachedClient, err := client.New(
		mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme(), Mapper: mgr.GetRESTMapper()})
	if err != nil {
		return fmt.Errorf("unable to set up uncached client: %w", err)
	}

	// Lookup ClusterID prior to starting
	cv := &configv1.ClusterVersion{}
	if err := uncachedClient.Get(ctx, client.ObjectKey{Name: "version"}, cv); err != nil {
		return fmt.Errorf("getting clusterversion: %w", err)
	}
	// calling this external ID to differentiate it from the cluster ID we use to contact OCM
	clusterExternalID := string(cv.Spec.ClusterID)

	// Create metrics recorder
	var recorder *metrics.Recorder
	if enableRecorder {
		recorder = metrics.NewRecorder(true, clusterExternalID)
	}

	addonReconciler := addoncontroller.NewAddonReconciler(
		mgr.GetClient(),
		uncachedClient,
		ctrl.Log.WithName("controllers").WithName("Addon"),
		mgr.GetScheme(),
		recorder,
		clusterExternalID,
		namespace,
		enableStatusReporting,
		opts...,
	)
	if err := addonReconciler.SetupWithManager(mgr, opts...); err != nil {
		return fmt.Errorf("unable to create Addon controller: %w", err)
	}

	if err := (&aocontroller.AddonOperatorReconciler{
		Client:              mgr.GetClient(),
		UncachedClient:      uncachedClient,
		Log:                 ctrl.Log.WithName("controllers").WithName("AddonOperator"),
		Scheme:              mgr.GetScheme(),
		GlobalPauseManager:  addonReconciler,
		OCMClientManager:    addonReconciler,
		Recorder:            recorder,
		ClusterExternalID:   clusterExternalID,
		FeatureTogglesState: strings.Split(addonOperatorInCluster.Spec.FeatureFlags, ","),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create AddonOperator controller: %w", err)
	}

	var (
		addonInstanceCtrlLog  = ctrl.Log.WithName("controllers").WithName("AddonInstance")
		addonInstancePhaseLog = addonInstanceCtrlLog.V(1).WithName("phase")
	)

	addonInstanceCtrl := aictrl.NewController(
		mgr.GetClient(),
		aictrl.WithLog{Log: addonInstanceCtrlLog},
		aictrl.WithSerialPhases{
			aictrl.NewPhaseCheckHeartbeat(
				aictrl.WithLog{Log: addonInstancePhaseLog.WithName("checkHeartbeat")},
			),
		},
		aictrl.WithRecorder{Recorder: recorder},
	)

	if err := addonInstanceCtrl.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up AddonInstance controller: %w", err)
	}

	return nil
}

func initPprof(mgr ctrl.Manager, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	s := &http.Server{
		Addr: addr, Handler: mux,
		// Mitigate: G112: Potential Slowloris Attack because
		// ReadHeaderTimeout is not configured in the http.Server (gosec)
		ReadHeaderTimeout: 2 * time.Second,
	}
	err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		errCh := make(chan error)
		defer func() {
			for range errCh {
			} // drain errCh for GC
		}()
		go func() {
			defer close(errCh)
			errCh <- s.ListenAndServe()
		}()

		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			s.Close()
			return nil
		}
	}))
	if err != nil {
		setupLog.Error(err, "unable to create pprof server")
		os.Exit(1)
	}
}

func fetchMetricsOptions(opts options) server.Options {
	metricsOpts := server.Options{
		BindAddress: opts.MetricsAddr,
	}

	if opts.MetricsTlsDir != "" {
		metricsOpts.SecureServing = true
		metricsOpts.CertDir = opts.MetricsTlsDir
	}

	return metricsOpts
}

func setup() error {
	// Create a client that does not cache resources cluster-wide.
	uncachedClient, err := client.New(
		ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("unable to set up uncached client: %w", err)
	}

	ctx := context.Background()

	addonOperatorObjectInCluster := addonsv1alpha1.AddonOperator{}
	if err := uncachedClient.Get(ctx, types.NamespacedName{Name: addonsv1alpha1.DefaultAddonOperatorName}, &addonOperatorObjectInCluster); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to GET the AddonOperator object: %w", err)
		}
		addonOperatorObjectInCluster = addonsv1alpha1.AddonOperator{}
	}

	addonReconcilerOptions := []addoncontroller.AddonReconcilerOptions{}

	// feature toggle handlers ADO intends to support
	featureToggleHandlers := featuretoggle.GetAvailableFeatureToggles(
		featuretoggle.WithSchemeToUpdate{Scheme: scheme},
		featuretoggle.WithAddonReconcilerOptsToUpdate{AddonReconcilerOptsToUpdate: &addonReconcilerOptions},
	)

	for _, featureToggleHandler := range featureToggleHandlers {
		if !featuretoggle.IsEnabled(featureToggleHandler, addonOperatorObjectInCluster) {
			continue
		}
		if err := featureToggleHandler.PreManagerSetupHandle(ctx); err != nil {
			return fmt.Errorf("failed to handle the feature '%s' before the manager's creation", featureToggleHandler.Name())
		}
	}

	opts := options{
		MetricsAddr:           ":8080",
		ProbeAddr:             ":8081",
		EnableMetricsRecorder: true,
		// Enable pprof by default to listen on localhost only.
		// This way we don't expose pprof open to the whole cluster we are running on,
		// while keeping it easy to access.
		// Example Command:
		// $ kubectl exec -it <addon-operator-pod> --container manager bash -- \
		// curl -sK -v http://localhost:8070/debug/pprof/heap > heap.out
		PprofAddr: "127.0.0.1:8070",
	}

	if err := opts.Process(); err != nil {
		return fmt.Errorf("processing options: %w", err)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                fetchMetricsOptions(opts),
		HealthProbeBindAddress: opts.ProbeAddr,
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port: 9443,
			}),
		LeaderElectionResourceLock: "leases",
		LeaderElection:             opts.EnableLeaderElection,
		LeaderElectionID:           "8a4hp84a6s.addon-operator-lock",
		LeaderElectionNamespace:    opts.LeaderElectionNamespace,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Secret{}: {
					Label: labels.SelectorFromSet(labels.Set{
						controllers.CommonCacheLabel: controllers.CommonCacheValue,
					}),
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	for _, featureToggleHandler := range featureToggleHandlers {
		if !featuretoggle.IsEnabled(featureToggleHandler, addonOperatorObjectInCluster) {
			continue
		}
		if err := featureToggleHandler.PostManagerSetupHandle(ctx, mgr); err != nil {
			return fmt.Errorf("failed to handle the feature '%s' after the manager's creation", featureToggleHandler.Name())
		}
	}

	// PPROF
	if len(opts.PprofAddr) > 0 {
		initPprof(mgr, opts.PprofAddr)
	}

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	if err := initReconcilers(mgr, opts.Namespace,
		opts.EnableMetricsRecorder, addonOperatorObjectInCluster, opts.StatusReportingEnabled, addonReconcilerOptions...); err != nil {
		return fmt.Errorf("init reconcilers: %w", err)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("problem running manager: %w", err)
	}
	return nil
}

func main() {
	if err := setup(); err != nil {
		setupLog.Error(err, "setting up manager")
		os.Exit(1)
	}
}

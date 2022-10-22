package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/selection"

	"github.com/openshift/addon-operator/internal/controllers"
	"github.com/openshift/addon-operator/internal/metrics"

	configv1 "github.com/openshift/api/config/v1"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	aoapis "github.com/openshift/addon-operator/apis"
	addoncontroller "github.com/openshift/addon-operator/internal/controllers/addon"
	aicontroller "github.com/openshift/addon-operator/internal/controllers/addoninstance"
	aocontroller "github.com/openshift/addon-operator/internal/controllers/addonoperator"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = aoapis.AddToScheme(scheme)
	_ = operatorsv1.AddToScheme(scheme)
	_ = operatorsv1alpha1.AddToScheme(scheme)
	_ = configv1.AddToScheme(scheme)
	_ = monitoringv1.AddToScheme(scheme)
}

func initReconcilers(mgr ctrl.Manager, namespace string, enableRecorder bool) error {
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
	)
	if err := addonReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create Addon controller: %w", err)
	}

	if err := (&aocontroller.AddonOperatorReconciler{
		Client:             mgr.GetClient(),
		UncachedClient:     uncachedClient,
		Log:                ctrl.Log.WithName("controllers").WithName("AddonOperator"),
		Scheme:             mgr.GetScheme(),
		GlobalPauseManager: addonReconciler,
		OCMClientManager:   addonReconciler,
		Recorder:           recorder,
		ClusterExternalID:  clusterExternalID,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create AddonOperator controller: %w", err)
	}

	if err := (&aicontroller.AddonInstanceReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controller").WithName("AddonInstance"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create AddonInstance controller: %w", err)
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

func setup() error {
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

	newCacheFunc, err := prepareCache()
	if err != nil {
		return fmt.Errorf("preparing cache: %w", err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         opts.MetricsAddr,
		HealthProbeBindAddress:     opts.ProbeAddr,
		Port:                       9443,
		LeaderElectionResourceLock: "leases",
		LeaderElection:             opts.EnableLeaderElection,
		LeaderElectionID:           "8a4hp84a6s.addon-operator-lock",
		LeaderElectionNamespace:    opts.LeaderElectionNamespace,
		NewCache:                   newCacheFunc,
	})
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
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

	if err := initReconcilers(mgr, opts.Namespace, opts.EnableMetricsRecorder); err != nil {
		return fmt.Errorf("init reconcilers: %w", err)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("problem running manager: %w", err)
	}
	return nil
}

func prepareCache() (cache.NewCacheFunc, error) {
	const olmCopiedFromLabel = "olm.copiedFrom"

	csvRequirement, err := labels.NewRequirement(olmCopiedFromLabel, selection.DoesNotExist, []string{})
	if err != nil {
		return nil, fmt.Errorf("creating CSV label selector requirement: %w", err)
	}

	opts := cache.Options{
		SelectorsByObject: cache.SelectorsByObject{
			&operatorsv1alpha1.ClusterServiceVersion{}: {
				Label: labels.NewSelector().Add(*csvRequirement),
			},
			&corev1.Secret{}: {
				Label: labels.SelectorFromSet(labels.Set{
					controllers.CommonCacheLabel: controllers.CommonCacheValue,
				}),
			},
		},
		TransformByObject: cache.TransformByObject{
			&operatorsv1alpha1.ClusterServiceVersion{}: stripUnusedCSVFields,
		},
	}

	return cache.BuilderWithOptions(opts), nil
}

var errInvalidObject = errors.New("invalid object")

func stripUnusedCSVFields(obj interface{}) (interface{}, error) {
	csv, ok := obj.(*operatorsv1alpha1.ClusterServiceVersion)
	if !ok {
		return nil, fmt.Errorf("casting %T as 'ClusterServiceVersion': %w", obj, errInvalidObject)
	}

	return &operatorsv1alpha1.ClusterServiceVersion{
		TypeMeta: csv.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:      csv.Name,
			Namespace: csv.Namespace,
		},
		Status: operatorsv1alpha1.ClusterServiceVersionStatus{
			Phase: csv.Status.Phase,
		},
	}, nil
}

func main() {
	if err := setup(); err != nil {
		setupLog.Error(err, "setting up manager")
		os.Exit(1)
	}
}

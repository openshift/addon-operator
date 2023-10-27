package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	addonsv1alpha1 "github.com/openshift/addon-operator/api/v1alpha1"
	"github.com/openshift/addon-operator/internal/webhooks"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = addonsv1alpha1.AddToScheme(scheme)
}

func main() {
	var (
		port      int
		certDir   string
		probeAddr string
	)

	flag.IntVar(&port, "port", 8080, "The port the webhook server binds to")
	flag.StringVar(&certDir, "cert-dir", "",
		"The directory that contains the server key and certificate")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: server.Options{BindAddress: "0"},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port:    port,
				CertDir: certDir,
			}),
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	setupLog.Info("Setting up webhook server")

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Register webhooks as handlers
	wbServer := mgr.GetWebhookServer()

	wbHandler := &webhooks.AddonWebhookHandler{
		Log:    log.Log.WithName("validating webhooks").WithName("Addon"),
		Client: mgr.GetClient(),
	}

	if err = wbHandler.InjectDecoder(admission.NewDecoder(mgr.GetScheme())); err != nil {
		setupLog.Error(err, "unable to inject decoder")
		os.Exit(1)
	}

	wbServer.Register("/validate-addon", &webhook.Admission{
		Handler: wbHandler,
	})

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

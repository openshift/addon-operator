package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

type options struct {
	EnableLeaderElection    bool
	EnableMetricsRecorder   bool
	LeaderElectionNamespace string
	MetricsAddr             string
	MetricsCertDir          string
	Namespace               string
	PprofAddr               string
	ProbeAddr               string
	StatusReportingEnabled  bool
}

// Process retrieves values from flags, environment values,
// and secret files in order and then validates the provided
// values to determine if any are invalid.
func (o *options) Process() error {
	o.parseFlags()
	o.processEnv()
	o.processSecrets()

	o.applyValuesFromOptions()

	return o.validate()
}

func (o *options) parseFlags() {
	flag.BoolVar(
		&o.EnableLeaderElection,
		"enable-leader-election",
		o.EnableLeaderElection,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.",
	)

	flag.BoolVar(
		&o.EnableMetricsRecorder,
		"enable-metrics-recorder",
		o.EnableMetricsRecorder,
		"Enable recording Addon Metrics",
	)

	flag.StringVar(
		&o.LeaderElectionNamespace,
		"leader-election-namspace",
		o.LeaderElectionNamespace,
		"The namespace in which the Leader Election resource wil be created.",
	)

	flag.StringVar(
		&o.MetricsAddr,
		"metrics-addr",
		o.MetricsAddr,
		"The address the metric endpoint binds to.",
	)

	flag.StringVar(
		&o.MetricsCertDir,
		"metrics-cert-dir",
		o.MetricsCertDir,
		strings.Join([]string{
			"The directory containing the TLS certificate (tls.crt) and key (tls.key) for secure metrics serviing.",
			"If unset metrics will be served without TLS.",
		}, " "),
	)

	flag.StringVar(
		&o.Namespace,
		"namespace",
		o.Namespace,
		"The namespace in which the operator is running.",
	)

	flag.StringVar(
		&o.PprofAddr,
		"pprof-addr", o.PprofAddr,
		"The address the pprof web endpoint binds to.",
	)

	flag.StringVar(
		&o.ProbeAddr,
		"health-probe-bind-address",
		o.ProbeAddr,
		"The address the probe endpoint binds to.",
	)

	flag.Parse()
}

func (o *options) processEnv() {
	if ns := os.Getenv("ADDON_OPERATOR_NAMESPACE"); ns != "" {
		if o.Namespace == "" {
			o.Namespace = ns
		}
	}

	if ns := os.Getenv("ADDON_OPERATOR_LE_NAMESPACE"); ns != "" {
		if o.LeaderElectionNamespace == "" {
			o.LeaderElectionNamespace = ns
		}
	}
	enableStatusReporting, ok := os.LookupEnv("ENABLE_STATUS_REPORTING")
	if ok && enableStatusReporting == "true" {
		o.StatusReportingEnabled = true
	} else {
		o.StatusReportingEnabled = false
	}
}

func (o *options) processSecrets() {
	const (
		scrtsPath              = "/var/run/secrets"
		inClusterNamespacePath = scrtsPath + "/kubernetes.io/serviceaccount/namespace"
	)

	var namespace string

	if ns, err := os.ReadFile(inClusterNamespacePath); err == nil {
		// Avoid applying a garbage value if an error occurred
		namespace = string(ns)
	}

	if o.Namespace == "" {
		o.Namespace = namespace
	}
}

func (o *options) applyValuesFromOptions() {
	if o.LeaderElectionNamespace == "" {
		o.LeaderElectionNamespace = o.Namespace
	}
}

var errInvalidOption = errors.New("invalid option")

func (o *options) validate() error {
	if o.Namespace == "" {
		return fmt.Errorf("'Namespace' must not be empty: %w", errInvalidOption)
	}

	return nil
}

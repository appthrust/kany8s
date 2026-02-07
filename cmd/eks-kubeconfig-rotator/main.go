package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	eksplugincontroller "github.com/reoring/kany8s/internal/controller/plugin/eks"
	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var secureMetrics bool
	var enableHTTP2 bool
	var watchNamespace string
	var refreshBefore string
	var maxRefresh string
	var failureBackoff string

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true, "Serve metrics over HTTPS.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false, "Enable HTTP/2 for metrics server.")
	flag.StringVar(&watchNamespace, "watch-namespace", "", "Namespace to watch. Empty means all namespaces.")
	flag.StringVar(&refreshBefore, "refresh-before", "5m", "Rotate token this duration before expiration.")
	flag.StringVar(&maxRefresh, "max-refresh-interval", "10m", "Maximum reconcile interval while token is valid.")
	flag.StringVar(&failureBackoff, "failure-backoff", "30s", "Requeue interval when prerequisites are not ready.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	policy, err := parsePolicy(refreshBefore, maxRefresh, failureBackoff)
	if err != nil {
		setupLog.Error(err, "invalid requeue policy flags")
		os.Exit(1)
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			c.NextProtos = []string{"http/1.1"}
		})
	}

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	cacheOptions := cache.Options{}
	if watchNamespace != "" {
		cacheOptions.DefaultNamespaces = map[string]cache.Config{
			watchNamespace: {},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f6b95f95.cluster.x-k8s.io",
		Cache:                  cacheOptions,
		Client: client.Options{
			Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&eksplugincontroller.EKSKubeconfigRotatorReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("eks-kubeconfig-rotator"), //nolint:staticcheck
		Policy:   policy,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EKSKubeconfigRotator")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func parsePolicy(refreshBefore, maxRefresh, failureBackoff string) (coreeks.RequeuePolicy, error) {
	refreshBeforeDuration, err := time.ParseDuration(refreshBefore)
	if err != nil {
		return coreeks.RequeuePolicy{}, fmt.Errorf("parse refresh-before: %w", err)
	}
	maxRefreshDuration, err := time.ParseDuration(maxRefresh)
	if err != nil {
		return coreeks.RequeuePolicy{}, fmt.Errorf("parse max-refresh-interval: %w", err)
	}
	failureBackoffDuration, err := time.ParseDuration(failureBackoff)
	if err != nil {
		return coreeks.RequeuePolicy{}, fmt.Errorf("parse failure-backoff: %w", err)
	}

	return coreeks.RequeuePolicy{
		RefreshBefore:  refreshBeforeDuration,
		MaxRefresh:     maxRefreshDuration,
		FailureBackoff: failureBackoffDuration,
	}, nil
}

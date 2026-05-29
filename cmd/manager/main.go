package main

import (
	"flag"
	"os"
	"strings"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	"github.com/k1s-project/k1s-operator/internal/controller"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(operatorv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	options := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "k1s-operator.operator.k1s.io",
	}
	if namespaces := watchNamespaces(os.Getenv("WATCH_NAMESPACE")); len(namespaces) > 0 {
		options.Cache.DefaultNamespaces = map[string]cache.Config{}
		for _, namespace := range namespaces {
			options.Cache.DefaultNamespaces[namespace] = cache.Config{}
		}
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		ctrl.Log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controller.K1sClusterReconciler{Client: mgr.GetClient(), Reader: mgr.GetAPIReader(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create K1sCluster controller")
		os.Exit(1)
	}
	if err := (&controller.K1sExposureReconciler{Client: mgr.GetClient(), Reader: mgr.GetAPIReader(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create K1sExposure controller")
		os.Exit(1)
	}
	if err := (&controller.K1sAppReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create K1sApp controller")
		os.Exit(1)
	}
	if err := (&controller.K1sInferenceEndpointReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create K1sInferenceEndpoint controller")
		os.Exit(1)
	}
	if err := (&controller.K1sResourceSetReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "unable to create K1sResourceSet controller")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	ctrl.Log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "manager exited")
		os.Exit(1)
	}
}

func watchNamespaces(raw string) []string {
	var namespaces []string
	for _, namespace := range strings.Split(raw, ",") {
		namespace = strings.TrimSpace(namespace)
		if namespace != "" {
			namespaces = append(namespaces, namespace)
		}
	}
	return namespaces
}

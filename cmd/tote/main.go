package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/ppiankov/tote/internal/config"
	"github.com/ppiankov/tote/internal/controller"
	"github.com/ppiankov/tote/internal/events"
	"github.com/ppiankov/tote/internal/inventory"
	"github.com/ppiankov/tote/internal/metrics"
	"github.com/ppiankov/tote/internal/version"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var enabled bool
	var metricsAddr string

	root := &cobra.Command{
		Use:     "tote",
		Short:   "Emergency image pull failure detector for Kubernetes",
		Version: version.Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(enabled, metricsAddr)
		},
		SilenceUsage: true,
	}

	root.Flags().BoolVar(&enabled, "enabled", true, "global kill switch for the operator")
	root.Flags().StringVar(&metricsAddr, "metrics-addr", ":8080", "address for the metrics endpoint")
	return root
}

func run(enabled bool, metricsAddr string) error {
	ctrl.SetLogger(zap.New())

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	cfg := config.New()
	cfg.Enabled = enabled

	reconciler := &controller.PodReconciler{
		Client:  mgr.GetClient(),
		Config:  cfg,
		Finder:  inventory.NewFinder(mgr.GetClient()),
		Emitter: events.NewEmitter(mgr.GetEventRecorder("tote")),
		Metrics: metrics.NewCounters(ctrlmetrics.Registry),
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}

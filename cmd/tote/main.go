package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/ppiankov/tote/internal/agent"
	"github.com/ppiankov/tote/internal/config"
	"github.com/ppiankov/tote/internal/controller"
	"github.com/ppiankov/tote/internal/events"
	"github.com/ppiankov/tote/internal/inventory"
	"github.com/ppiankov/tote/internal/metrics"
	"github.com/ppiankov/tote/internal/session"
	"github.com/ppiankov/tote/internal/transfer"
	"github.com/ppiankov/tote/internal/version"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "tote",
		Short:        "Emergency image pull failure detector and salvager for Kubernetes",
		Version:      version.Version,
		SilenceUsage: true,
	}

	controllerCmd := newControllerCmd()
	agentCmd := newAgentCmd()

	root.AddCommand(controllerCmd)
	root.AddCommand(agentCmd)

	// Bare "tote" (no subcommand) runs the controller for backward compat.
	root.RunE = controllerCmd.RunE

	// Copy controller flags to root for backward compat.
	root.Flags().AddFlagSet(controllerCmd.Flags())

	return root
}

func newControllerCmd() *cobra.Command {
	var (
		enabled               bool
		metricsAddr           string
		maxConcurrentSalvages int
		sessionTTL            string
		agentNamespace        string
		agentGRPCPort         int
	)

	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Run the tote controller (detects failures, orchestrates salvage)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runController(enabled, metricsAddr, maxConcurrentSalvages, sessionTTL, agentNamespace, agentGRPCPort)
		},
	}

	cmd.Flags().BoolVar(&enabled, "enabled", true, "global kill switch for the operator")
	cmd.Flags().StringVar(&metricsAddr, "metrics-addr", ":8080", "address for the metrics endpoint")
	cmd.Flags().IntVar(&maxConcurrentSalvages, "max-concurrent-salvages", config.DefaultMaxConcurrentSalvages, "max parallel salvage operations")
	cmd.Flags().StringVar(&sessionTTL, "session-ttl", config.DefaultSessionTTL.String(), "session lifetime for salvage operations")
	cmd.Flags().StringVar(&agentNamespace, "agent-namespace", "", "namespace where tote agents run (required for salvage)")
	cmd.Flags().IntVar(&agentGRPCPort, "agent-grpc-port", config.DefaultAgentGRPCPort, "gRPC port for agent communication")

	return cmd
}

func newAgentCmd() *cobra.Command {
	var (
		containerdSocket string
		grpcPort         int
		metricsAddr      string
	)

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run the tote agent (serves images from local containerd)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(containerdSocket, grpcPort, metricsAddr)
		},
	}

	cmd.Flags().StringVar(&containerdSocket, "containerd-socket", config.DefaultContainerdSocket, "path to containerd socket")
	cmd.Flags().IntVar(&grpcPort, "grpc-port", config.DefaultAgentGRPCPort, "gRPC listen port")
	cmd.Flags().StringVar(&metricsAddr, "metrics-addr", ":8081", "address for the metrics endpoint")

	return cmd
}

func runController(enabled bool, metricsAddr string, maxConcurrentSalvages int, sessionTTLStr, agentNamespace string, agentGRPCPort int) error {
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
	cfg.AgentNamespace = agentNamespace
	cfg.AgentGRPCPort = agentGRPCPort
	cfg.MaxConcurrentSalvages = maxConcurrentSalvages

	sessionTTL := config.DefaultSessionTTL
	if sessionTTLStr != "" && sessionTTLStr != config.DefaultSessionTTL.String() {
		parsed, err := time.ParseDuration(sessionTTLStr)
		if err != nil {
			return fmt.Errorf("invalid session-ttl: %w", err)
		}
		sessionTTL = parsed
	}
	cfg.SessionTTL = sessionTTL

	m := metrics.NewCounters(ctrlmetrics.Registry)
	emitter := events.NewEmitter(mgr.GetEventRecorder("tote"))

	reconciler := &controller.PodReconciler{
		Client:  mgr.GetClient(),
		Config:  cfg,
		Finder:  inventory.NewFinder(mgr.GetClient()),
		Emitter: emitter,
		Metrics: m,
	}

	// Set up salvage orchestrator if agent namespace is configured.
	if agentNamespace != "" {
		sessions := session.NewStore()
		resolver := transfer.NewResolver(mgr.GetClient(), agentNamespace, agentGRPCPort)
		reconciler.AgentResolver = resolver
		reconciler.Orchestrator = transfer.NewOrchestrator(
			sessions, resolver, emitter, m, mgr.GetClient(),
			maxConcurrentSalvages, sessionTTL,
		)
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}

func runAgent(containerdSocket string, grpcPort int, metricsAddr string) error {
	ctrl.SetLogger(zap.New())
	logger := ctrl.Log.WithName("agent")

	// Hard fail if containerd socket is not accessible.
	if _, err := os.Stat(containerdSocket); err != nil {
		return fmt.Errorf("containerd socket %s: %w (agent requires containerd access)", containerdSocket, err)
	}

	store, err := agent.NewContainerdStore(containerdSocket)
	if err != nil {
		return fmt.Errorf("connecting to containerd: %w", err)
	}
	defer func() { _ = store.Close() }()

	sessions := session.NewStore()
	srv := agent.NewServer(store, sessions, grpcPort)

	logger.Info("starting agent", "grpc-port", grpcPort, "containerd-socket", containerdSocket, "metrics-addr", metricsAddr)
	return srv.Start(ctrl.SetupSignalHandler())
}

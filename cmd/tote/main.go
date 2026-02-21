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
	"github.com/ppiankov/tote/internal/tlsutil"
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
		enabled                bool
		metricsAddr            string
		maxConcurrentSalvages  int
		sessionTTL             string
		agentNamespace         string
		agentGRPCPort          int
		maxImageSize           int64
		backupRegistry         string
		backupRegistrySecret   string
		backupRegistryInsecure bool
		tlsCert                string
		tlsKey                 string
		tlsCA                  string
	)

	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Run the tote controller (detects failures, orchestrates salvage)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.ValidateTLSFlags(tlsCert, tlsKey, tlsCA); err != nil {
				return err
			}
			return runController(enabled, metricsAddr, maxConcurrentSalvages, sessionTTL, agentNamespace, agentGRPCPort, maxImageSize, backupRegistry, backupRegistrySecret, backupRegistryInsecure, tlsCert, tlsKey, tlsCA)
		},
	}

	cmd.Flags().BoolVar(&enabled, "enabled", true, "global kill switch for the operator")
	cmd.Flags().StringVar(&metricsAddr, "metrics-addr", ":8080", "address for the metrics endpoint")
	cmd.Flags().IntVar(&maxConcurrentSalvages, "max-concurrent-salvages", config.DefaultMaxConcurrentSalvages, "max parallel salvage operations")
	cmd.Flags().StringVar(&sessionTTL, "session-ttl", config.DefaultSessionTTL.String(), "session lifetime for salvage operations")
	cmd.Flags().StringVar(&agentNamespace, "agent-namespace", "", "namespace where tote agents run (required for salvage)")
	cmd.Flags().IntVar(&agentGRPCPort, "agent-grpc-port", config.DefaultAgentGRPCPort, "gRPC port for agent communication")
	cmd.Flags().Int64Var(&maxImageSize, "max-image-size", config.DefaultMaxImageSize, "max image size in bytes for salvage (0 = no limit)")
	cmd.Flags().StringVar(&backupRegistry, "backup-registry", "", "registry host to push salvaged images (empty = disabled)")
	cmd.Flags().StringVar(&backupRegistrySecret, "backup-registry-secret", "", "name of dockerconfigjson Secret for backup registry credentials")
	cmd.Flags().BoolVar(&backupRegistryInsecure, "backup-registry-insecure", false, "allow HTTP connections to backup registry")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "path to TLS certificate file (enables mTLS when all three TLS flags are set)")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "path to TLS private key file")
	cmd.Flags().StringVar(&tlsCA, "tls-ca", "", "path to CA certificate file")

	return cmd
}

func newAgentCmd() *cobra.Command {
	var (
		containerdSocket string
		grpcPort         int
		metricsAddr      string
		tlsCert          string
		tlsKey           string
		tlsCA            string
	)

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run the tote agent (serves images from local containerd)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.ValidateTLSFlags(tlsCert, tlsKey, tlsCA); err != nil {
				return err
			}
			return runAgent(containerdSocket, grpcPort, metricsAddr, tlsCert, tlsKey, tlsCA)
		},
	}

	cmd.Flags().StringVar(&containerdSocket, "containerd-socket", config.DefaultContainerdSocket, "path to containerd socket")
	cmd.Flags().IntVar(&grpcPort, "grpc-port", config.DefaultAgentGRPCPort, "gRPC listen port")
	cmd.Flags().StringVar(&metricsAddr, "metrics-addr", ":8081", "address for the metrics endpoint")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "path to TLS certificate file (enables mTLS when all three TLS flags are set)")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "path to TLS private key file")
	cmd.Flags().StringVar(&tlsCA, "tls-ca", "", "path to CA certificate file")

	return cmd
}

func runController(enabled bool, metricsAddr string, maxConcurrentSalvages int, sessionTTLStr, agentNamespace string, agentGRPCPort int, maxImageSize int64, backupRegistry, backupRegistrySecret string, backupRegistryInsecure bool, tlsCert, tlsKey, tlsCA string) error {
	ctrl.SetLogger(zap.New())

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:   true,
		LeaderElectionID: "tote-controller",
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	cfg := config.New()
	cfg.Enabled = enabled
	cfg.AgentNamespace = agentNamespace
	cfg.AgentGRPCPort = agentGRPCPort
	cfg.MaxConcurrentSalvages = maxConcurrentSalvages
	cfg.MaxImageSize = maxImageSize

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

		// Load mTLS client credentials for agent communication.
		if config.TLSEnabled(tlsCert, tlsKey, tlsCA) {
			clientCreds, err := tlsutil.ClientCredentials(tlsCert, tlsKey, tlsCA)
			if err != nil {
				return fmt.Errorf("loading TLS credentials: %w", err)
			}
			resolver.TransportCreds = clientCreds
		}

		reconciler.AgentResolver = resolver
		orch := transfer.NewOrchestrator(
			sessions, resolver, emitter, m, mgr.GetClient(),
			maxConcurrentSalvages, sessionTTL, maxImageSize,
		)
		orch.TransportCreds = resolver.TransportCreds
		if backupRegistry != "" {
			orch.SetBackupRegistry(backupRegistry, backupRegistrySecret, agentNamespace, backupRegistryInsecure)
		}
		reconciler.Orchestrator = orch
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}

func runAgent(containerdSocket string, grpcPort int, metricsAddr, tlsCert, tlsKey, tlsCA string) error {
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

	if config.TLSEnabled(tlsCert, tlsKey, tlsCA) {
		serverCreds, err := tlsutil.ServerCredentials(tlsCert, tlsKey, tlsCA)
		if err != nil {
			return fmt.Errorf("loading server TLS credentials: %w", err)
		}
		clientCreds, err := tlsutil.ClientCredentials(tlsCert, tlsKey, tlsCA)
		if err != nil {
			return fmt.Errorf("loading client TLS credentials: %w", err)
		}
		srv.ServerCreds = serverCreds
		srv.ClientCreds = clientCreds
		logger.Info("mTLS enabled")
	}

	logger.Info("starting agent", "grpc-port", grpcPort, "containerd-socket", containerdSocket, "metrics-addr", metricsAddr)
	return srv.Start(ctrl.SetupSignalHandler())
}

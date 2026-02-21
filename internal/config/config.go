package config

import "time"

const (
	// AnnotationNamespaceAllow is required on the Namespace for tote to act.
	AnnotationNamespaceAllow = "tote.dev/allow"

	// AnnotationPodAutoSalvage is required on the Pod.
	AnnotationPodAutoSalvage = "tote.dev/auto-salvage"

	// AnnotationSalvagedDigest marks a pod as already salvaged for a digest.
	AnnotationSalvagedDigest = "tote.dev/salvaged-digest"

	// AnnotationImportedAt records when the salvage completed.
	AnnotationImportedAt = "tote.dev/imported-at"

	// DefaultContainerdSocket is the default containerd socket path.
	DefaultContainerdSocket = "/run/containerd/containerd.sock"

	// DefaultAgentGRPCPort is the default gRPC port for the agent.
	DefaultAgentGRPCPort = 9090

	// DefaultMaxConcurrentSalvages is the default concurrent salvage limit.
	DefaultMaxConcurrentSalvages = 2

	// DefaultSessionTTL is the default session lifetime.
	DefaultSessionTTL = 5 * time.Minute

	// DefaultMaxImageSize is the default max image size for salvage (2 GiB).
	DefaultMaxImageSize int64 = 2 * 1024 * 1024 * 1024
)

// DefaultDeniedNamespaces are always excluded regardless of annotations.
var DefaultDeniedNamespaces = []string{
	"kube-system",
	"kube-public",
	"kube-node-lease",
}

// Config holds operator runtime configuration.
type Config struct {
	// Enabled is the global kill switch. When false, reconciler is a no-op.
	Enabled bool

	// DeniedNamespaces are namespaces that are never processed.
	DeniedNamespaces map[string]bool

	// MaxConcurrentSalvages limits parallel salvage operations.
	MaxConcurrentSalvages int

	// SessionTTL is the lifetime for salvage sessions.
	SessionTTL time.Duration

	// AgentNamespace is the namespace where tote agents run.
	AgentNamespace string

	// AgentGRPCPort is the gRPC port for agents.
	AgentGRPCPort int

	// MaxImageSize is the max image size in bytes for salvage. 0 means no limit.
	MaxImageSize int64

	// BackupRegistry is the registry host to push salvaged images to.
	// Empty means registry push is disabled.
	BackupRegistry string

	// BackupRegistrySecret is the name of the k8s Secret (dockerconfigjson)
	// containing credentials for the backup registry.
	BackupRegistrySecret string

	// BackupRegistryInsecure allows HTTP connections to the backup registry.
	BackupRegistryInsecure bool
}

// AgentConfig holds agent-specific configuration.
type AgentConfig struct {
	ContainerdSocket string
	GRPCPort         int
	MetricsAddr      string
}

// New creates a Config with default values.
func New() Config {
	denied := make(map[string]bool, len(DefaultDeniedNamespaces))
	for _, ns := range DefaultDeniedNamespaces {
		denied[ns] = true
	}
	return Config{
		Enabled:               true,
		DeniedNamespaces:      denied,
		MaxConcurrentSalvages: DefaultMaxConcurrentSalvages,
		SessionTTL:            DefaultSessionTTL,
		AgentGRPCPort:         DefaultAgentGRPCPort,
		MaxImageSize:          DefaultMaxImageSize,
	}
}

// IsDenied returns true if the namespace must not be processed.
func (c Config) IsDenied(namespace string) bool {
	return c.DeniedNamespaces[namespace]
}

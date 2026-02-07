package config

const (
	// AnnotationNamespaceAllow is required on the Namespace for tote to act.
	AnnotationNamespaceAllow = "tote.dev/allow"

	// AnnotationPodAutoSalvage is required on the Pod.
	AnnotationPodAutoSalvage = "tote.dev/auto-salvage"
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
}

// New creates a Config with default values.
func New() Config {
	denied := make(map[string]bool, len(DefaultDeniedNamespaces))
	for _, ns := range DefaultDeniedNamespaces {
		denied[ns] = true
	}
	return Config{
		Enabled:          true,
		DeniedNamespaces: denied,
	}
}

// IsDenied returns true if the namespace must not be processed.
func (c Config) IsDenied(namespace string) bool {
	return c.DeniedNamespaces[namespace]
}

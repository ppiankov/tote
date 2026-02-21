package registry

import (
	"fmt"
	"strings"
)

// BackupRef replaces the registry host in an image reference with the backup
// registry host. Digest suffixes are stripped (the push creates its own digest).
//
// Examples:
//
//	BackupRef("registry.example.com/team/app:v1", "backup.example.com:5000")
//	  → "backup.example.com:5000/team/app:v1"
//	BackupRef("nginx:latest", "backup.example.com:5000")
//	  → "backup.example.com:5000/nginx:latest"
func BackupRef(originalRef, backupRegistry string) (string, error) {
	if backupRegistry == "" {
		return "", fmt.Errorf("backup registry is empty")
	}

	ref := originalRef

	// Strip digest suffix — push creates its own digest.
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}

	// If no tag after stripping digest, add :latest so the ref is valid.
	if !strings.Contains(ref, ":") || (strings.Contains(ref, "/") && !strings.Contains(ref[strings.LastIndex(ref, "/"):], ":")) {
		ref += ":latest"
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) < 2 {
		// Single-component image like "nginx:latest".
		return backupRegistry + "/" + ref, nil
	}

	// If first part looks like a hostname (contains . or :), replace it.
	if strings.ContainsAny(parts[0], ".:") {
		return backupRegistry + "/" + parts[1], nil
	}

	// No host prefix (e.g., "library/nginx:latest").
	return backupRegistry + "/" + ref, nil
}

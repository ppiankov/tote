package config

import (
	"testing"
	"time"
)

func TestNew_DefaultsEnabled(t *testing.T) {
	cfg := New()
	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
}

func TestNew_SalvageDefaults(t *testing.T) {
	cfg := New()
	if cfg.MaxConcurrentSalvages != DefaultMaxConcurrentSalvages {
		t.Errorf("expected MaxConcurrentSalvages=%d, got %d", DefaultMaxConcurrentSalvages, cfg.MaxConcurrentSalvages)
	}
	if cfg.SessionTTL != 5*time.Minute {
		t.Errorf("expected SessionTTL=5m, got %v", cfg.SessionTTL)
	}
	if cfg.AgentGRPCPort != DefaultAgentGRPCPort {
		t.Errorf("expected AgentGRPCPort=%d, got %d", DefaultAgentGRPCPort, cfg.AgentGRPCPort)
	}
}

func TestNew_DefaultDeniedNamespaces(t *testing.T) {
	cfg := New()
	for _, ns := range DefaultDeniedNamespaces {
		if !cfg.IsDenied(ns) {
			t.Errorf("expected %q to be denied by default", ns)
		}
	}
}

func TestIsDenied_CustomNamespace(t *testing.T) {
	cfg := New()
	cfg.DeniedNamespaces["custom-deny"] = true
	if !cfg.IsDenied("custom-deny") {
		t.Error("expected custom-deny to be denied")
	}
}

func TestIsDenied_AllowedNamespace(t *testing.T) {
	cfg := New()
	if cfg.IsDenied("default") {
		t.Error("expected 'default' namespace to not be denied")
	}
}

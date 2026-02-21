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

func TestTLSEnabled(t *testing.T) {
	if TLSEnabled("", "", "") {
		t.Error("expected false when all empty")
	}
	if !TLSEnabled("/cert", "/key", "/ca") {
		t.Error("expected true when all set")
	}
	if TLSEnabled("/cert", "", "") {
		t.Error("expected false when partially set")
	}
}

func TestValidateTLSFlags(t *testing.T) {
	if err := ValidateTLSFlags("", "", ""); err != nil {
		t.Errorf("all empty should be valid: %v", err)
	}
	if err := ValidateTLSFlags("/cert", "/key", "/ca"); err != nil {
		t.Errorf("all set should be valid: %v", err)
	}
	if err := ValidateTLSFlags("/cert", "", ""); err == nil {
		t.Error("1 of 3 should be invalid")
	}
	if err := ValidateTLSFlags("/cert", "/key", ""); err == nil {
		t.Error("2 of 3 should be invalid")
	}
}

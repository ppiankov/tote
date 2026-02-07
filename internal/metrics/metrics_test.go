package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewCounters(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCounters(reg)
	if c.DetectedFailures == nil || c.SalvageableImages == nil || c.NotActionable == nil {
		t.Fatal("expected all counters to be initialized")
	}
}

func TestRecordDetected(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCounters(reg)
	c.RecordDetected()
	c.RecordDetected()
	val := testutil.ToFloat64(c.DetectedFailures)
	if val != 2 {
		t.Errorf("expected 2, got %f", val)
	}
}

func TestRecordSalvageable(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCounters(reg)
	c.RecordSalvageable()
	val := testutil.ToFloat64(c.SalvageableImages)
	if val != 1 {
		t.Errorf("expected 1, got %f", val)
	}
}

func TestRecordNotActionable(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCounters(reg)
	c.RecordNotActionable()
	c.RecordNotActionable()
	c.RecordNotActionable()
	val := testutil.ToFloat64(c.NotActionable)
	if val != 3 {
		t.Errorf("expected 3, got %f", val)
	}
}

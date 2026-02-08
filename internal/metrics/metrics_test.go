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
		t.Fatal("expected detection counters to be initialized")
	}
	if c.SalvageAttempts == nil || c.SalvageSuccesses == nil || c.SalvageFailures == nil {
		t.Fatal("expected salvage counters to be initialized")
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

func TestRecordSalvageAttempt(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCounters(reg)
	c.RecordSalvageAttempt()
	val := testutil.ToFloat64(c.SalvageAttempts)
	if val != 1 {
		t.Errorf("expected 1, got %f", val)
	}
}

func TestRecordSalvageSuccess(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCounters(reg)
	c.RecordSalvageSuccess()
	val := testutil.ToFloat64(c.SalvageSuccesses)
	if val != 1 {
		t.Errorf("expected 1, got %f", val)
	}
}

func TestRecordSalvageFailure(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCounters(reg)
	c.RecordSalvageFailure()
	c.RecordSalvageFailure()
	val := testutil.ToFloat64(c.SalvageFailures)
	if val != 2 {
		t.Errorf("expected 2, got %f", val)
	}
}

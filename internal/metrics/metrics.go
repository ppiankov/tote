package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Counters holds all tote Prometheus metrics.
type Counters struct {
	DetectedFailures  prometheus.Counter
	SalvageableImages prometheus.Counter
	NotActionable     prometheus.Counter
	CorruptImages     prometheus.Counter
	SalvageAttempts   prometheus.Counter
	SalvageSuccesses  prometheus.Counter
	SalvageFailures   prometheus.Counter
}

// NewCounters creates and registers Prometheus counters with the given registry.
func NewCounters(reg prometheus.Registerer) *Counters {
	c := &Counters{
		DetectedFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tote_detected_failures_total",
			Help: "Total number of image pull failures detected.",
		}),
		SalvageableImages: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tote_salvageable_images_total",
			Help: "Total number of failures where the image digest was found on cluster nodes.",
		}),
		NotActionable: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tote_not_actionable_total",
			Help: "Total number of failures where the image uses a tag instead of a digest.",
		}),
		CorruptImages: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tote_corrupt_images_total",
			Help: "Total number of corrupt image records detected and cleaned.",
		}),
		SalvageAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tote_salvage_attempts_total",
			Help: "Total number of image salvage attempts.",
		}),
		SalvageSuccesses: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tote_salvage_successes_total",
			Help: "Total number of successful image salvages.",
		}),
		SalvageFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tote_salvage_failures_total",
			Help: "Total number of failed image salvage attempts.",
		}),
	}

	reg.MustRegister(
		c.DetectedFailures,
		c.SalvageableImages,
		c.NotActionable,
		c.CorruptImages,
		c.SalvageAttempts,
		c.SalvageSuccesses,
		c.SalvageFailures,
	)

	return c
}

// RecordDetected increments the detected failures counter.
func (c *Counters) RecordDetected() {
	c.DetectedFailures.Inc()
}

// RecordSalvageable increments the salvageable images counter.
func (c *Counters) RecordSalvageable() {
	c.SalvageableImages.Inc()
}

// RecordNotActionable increments the not-actionable counter.
func (c *Counters) RecordNotActionable() {
	c.NotActionable.Inc()
}

// RecordCorruptImage increments the corrupt images counter.
func (c *Counters) RecordCorruptImage() {
	c.CorruptImages.Inc()
}

// RecordSalvageAttempt increments the salvage attempts counter.
func (c *Counters) RecordSalvageAttempt() {
	c.SalvageAttempts.Inc()
}

// RecordSalvageSuccess increments the salvage successes counter.
func (c *Counters) RecordSalvageSuccess() {
	c.SalvageSuccesses.Inc()
}

// RecordSalvageFailure increments the salvage failures counter.
func (c *Counters) RecordSalvageFailure() {
	c.SalvageFailures.Inc()
}

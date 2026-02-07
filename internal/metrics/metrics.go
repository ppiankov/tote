package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Counters holds all tote Prometheus metrics.
type Counters struct {
	DetectedFailures  prometheus.Counter
	SalvageableImages prometheus.Counter
	NotActionable     prometheus.Counter
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
	}

	reg.MustRegister(
		c.DetectedFailures,
		c.SalvageableImages,
		c.NotActionable,
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

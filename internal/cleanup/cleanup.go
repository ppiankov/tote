package cleanup

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/ppiankov/tote/api/v1alpha1"
)

// Reaper periodically deletes expired SalvageRecords.
// It implements manager.Runnable and manager.LeaderElectionRunnable.
type Reaper struct {
	Client   client.Client
	TTL      time.Duration
	Interval time.Duration
}

// NewReaper creates a Reaper with the given TTL and check interval.
func NewReaper(c client.Client, ttl, interval time.Duration) *Reaper {
	return &Reaper{
		Client:   c,
		TTL:      ttl,
		Interval: interval,
	}
}

// NeedLeaderElection returns true so cleanup only runs on the leader.
func (r *Reaper) NeedLeaderElection() bool {
	return true
}

// Start runs the periodic cleanup loop until ctx is cancelled.
func (r *Reaper) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("cleanup")
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.sweep(ctx, logger)
		}
	}
}

func (r *Reaper) sweep(ctx context.Context, logger interface{ Info(string, ...interface{}) }) {
	var list v1alpha1.SalvageRecordList
	if err := r.Client.List(ctx, &list); err != nil {
		return
	}

	cutoff := time.Now().Add(-r.TTL)
	for i := range list.Items {
		rec := &list.Items[i]
		if rec.Status.CompletedAt == "" {
			continue
		}
		completed, err := time.Parse(time.RFC3339, rec.Status.CompletedAt)
		if err != nil {
			continue
		}
		if completed.Before(cutoff) {
			if err := r.Client.Delete(ctx, rec); err == nil {
				logger.Info("deleted expired SalvageRecord", "name", rec.Name, "namespace", rec.Namespace)
			}
		}
	}
}

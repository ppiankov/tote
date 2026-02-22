package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Event represents a notification payload sent to webhooks.
type Event struct {
	Type       string `json:"type"`
	PodName    string `json:"pod_name"`
	Namespace  string `json:"namespace"`
	ImageRef   string `json:"image_ref,omitempty"`
	Digest     string `json:"digest,omitempty"`
	SourceNode string `json:"source_node,omitempty"`
	TargetNode string `json:"target_node,omitempty"`
	Error      string `json:"error,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// Notifier sends webhook notifications. Fire-and-forget with timeout.
type Notifier struct {
	URL        string
	Events     map[string]bool
	HTTPClient *http.Client
}

// NewNotifier creates a Notifier for the given URL and event filter.
// eventTypes is a list of event types to send (e.g. "detected", "salvaged").
// An empty list means all events are sent.
func NewNotifier(url string, eventTypes []string) *Notifier {
	events := make(map[string]bool)
	for _, e := range eventTypes {
		events[e] = true
	}
	return &Notifier{
		URL:    url,
		Events: events,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Notify sends an event to the webhook. Errors are logged but never returned
// to the caller (fire-and-forget).
func (n *Notifier) Notify(ctx context.Context, evt Event) error {
	if n == nil || n.URL == "" {
		return nil
	}
	if len(n.Events) > 0 && !n.Events[evt.Type] {
		return nil
	}

	evt.Timestamp = time.Now().UTC().Format(time.RFC3339)

	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

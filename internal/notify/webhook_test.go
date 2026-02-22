package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifier_SendsEvent(t *testing.T) {
	var received Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type application/json")
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, nil)
	err := n.Notify(context.Background(), Event{
		Type:      "salvaged",
		PodName:   "app-1",
		Namespace: "default",
		Digest:    "sha256:abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received.Type != "salvaged" {
		t.Errorf("expected type 'salvaged', got %q", received.Type)
	}
	if received.PodName != "app-1" {
		t.Errorf("expected pod 'app-1', got %q", received.PodName)
	}
	if received.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNotifier_FiltersEvents(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, []string{"salvaged"})

	// Filtered out event type.
	_ = n.Notify(context.Background(), Event{Type: "detected"})
	if called {
		t.Error("expected 'detected' event to be filtered out")
	}

	// Allowed event type.
	_ = n.Notify(context.Background(), Event{Type: "salvaged"})
	if !called {
		t.Error("expected 'salvaged' event to be sent")
	}
}

func TestNotifier_NilIsNoop(t *testing.T) {
	var n *Notifier
	err := n.Notify(context.Background(), Event{Type: "test"})
	if err != nil {
		t.Fatalf("nil notifier should be noop, got: %v", err)
	}
}

func TestNotifier_EmptyURLIsNoop(t *testing.T) {
	n := NewNotifier("", nil)
	err := n.Notify(context.Background(), Event{Type: "test"})
	if err != nil {
		t.Fatalf("empty URL should be noop, got: %v", err)
	}
}

func TestNotifier_ReturnsErrorOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, nil)
	err := n.Notify(context.Background(), Event{Type: "test"})
	if err == nil {
		t.Error("expected error on 500 status")
	}
}

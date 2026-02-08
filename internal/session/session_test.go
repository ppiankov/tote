package session

import (
	"testing"
	"time"
)

func TestCreate(t *testing.T) {
	s := NewStore()
	sess := s.Create("sha256:abc", "node-a", "node-b", 5*time.Minute)

	if sess.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if sess.Digest != "sha256:abc" {
		t.Errorf("expected digest sha256:abc, got %s", sess.Digest)
	}
	if sess.SourceNode != "node-a" {
		t.Errorf("expected source node-a, got %s", sess.SourceNode)
	}
	if sess.TargetNode != "node-b" {
		t.Errorf("expected target node-b, got %s", sess.TargetNode)
	}
	if sess.ExpiresAt.Before(time.Now()) {
		t.Error("expected expiry in the future")
	}
	if s.Len() != 1 {
		t.Errorf("expected 1 session, got %d", s.Len())
	}
}

func TestValidate_Valid(t *testing.T) {
	s := NewStore()
	sess := s.Create("sha256:abc", "node-a", "node-b", 5*time.Minute)

	got, ok := s.Validate(sess.Token)
	if !ok {
		t.Fatal("expected session to be valid")
	}
	if got.Digest != "sha256:abc" {
		t.Errorf("expected digest sha256:abc, got %s", got.Digest)
	}
}

func TestValidate_Expired(t *testing.T) {
	s := NewStore()
	sess := s.Create("sha256:abc", "node-a", "node-b", -1*time.Second)

	_, ok := s.Validate(sess.Token)
	if ok {
		t.Error("expected expired session to be invalid")
	}
	if s.Len() != 0 {
		t.Errorf("expected expired session to be cleaned up, got %d sessions", s.Len())
	}
}

func TestValidate_NotFound(t *testing.T) {
	s := NewStore()

	_, ok := s.Validate("nonexistent")
	if ok {
		t.Error("expected nonexistent token to be invalid")
	}
}

func TestDelete(t *testing.T) {
	s := NewStore()
	sess := s.Create("sha256:abc", "node-a", "node-b", 5*time.Minute)

	s.Delete(sess.Token)

	_, ok := s.Validate(sess.Token)
	if ok {
		t.Error("expected deleted session to be invalid")
	}
	if s.Len() != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", s.Len())
	}
}

func TestCleanup(t *testing.T) {
	s := NewStore()
	s.Create("sha256:abc", "node-a", "node-b", -1*time.Second)
	s.Create("sha256:def", "node-c", "node-d", -1*time.Second)
	s.Create("sha256:ghi", "node-e", "node-f", 5*time.Minute)

	s.Cleanup()

	if s.Len() != 1 {
		t.Errorf("expected 1 session after cleanup, got %d", s.Len())
	}
}

func TestCreate_UniqueTokens(t *testing.T) {
	s := NewStore()
	s1 := s.Create("sha256:abc", "node-a", "node-b", 5*time.Minute)
	s2 := s.Create("sha256:abc", "node-a", "node-b", 5*time.Minute)

	if s1.Token == s2.Token {
		t.Error("expected unique tokens for different sessions")
	}
}

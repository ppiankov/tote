package session

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session represents an authorized image transfer between two nodes.
type Session struct {
	Token      string
	Digest     string
	SourceNode string
	TargetNode string
	ExpiresAt  time.Time
}

// Store holds active sessions in memory. Thread-safe.
type Store struct {
	mu       sync.Mutex
	sessions map[string]Session
}

// NewStore creates an empty session store.
func NewStore() *Store {
	return &Store{sessions: make(map[string]Session)}
}

// Create registers a new session with the given parameters and TTL.
// Returns the created session with a generated token.
func (s *Store) Create(digest, sourceNode, targetNode string, ttl time.Duration) Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess := Session{
		Token:      uuid.New().String(),
		Digest:     digest,
		SourceNode: sourceNode,
		TargetNode: targetNode,
		ExpiresAt:  time.Now().Add(ttl),
	}
	s.sessions[sess.Token] = sess
	return sess
}

// Validate returns the session for the given token if it exists and has not
// expired. Expired sessions are deleted on access.
func (s *Store) Validate(token string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[token]
	if !ok {
		return Session{}, false
	}
	if time.Now().After(sess.ExpiresAt) {
		delete(s.sessions, token)
		return Session{}, false
	}
	return sess, true
}

// Delete removes a session by token.
func (s *Store) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

// Cleanup removes all expired sessions.
func (s *Store) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
}

// Len returns the number of active sessions. Intended for testing.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

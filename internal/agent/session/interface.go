package session

import (
	"time"

	"github.com/cloudwego/eino/schema"
)

// Store defines the interface for session storage implementations.
type Store interface {
	// Create creates a new session with the given ID and instruction.
	Create(sessionID, instruction string) error

	// Exists checks if a session exists.
	Exists(sessionID string) bool

	// GetMeta retrieves session metadata.
	GetMeta(sessionID string) (*SessionMeta, bool)

	// ListSessions returns metadata for all active (non-expired) sessions.
	ListSessions() []*SessionMeta

	// GetMessages retrieves all messages in a session.
	GetMessages(sessionID string) ([]*schema.Message, error)

	// Append adds messages to a session.
	Append(sessionID string, msgs ...*schema.Message) error

	// Delete removes a session.
	Delete(sessionID string)

	// StartCleanup starts background cleanup of expired sessions.
	// Returns a cancel function to stop cleanup.
	StartCleanup(interval time.Duration) func()
}

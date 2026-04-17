package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

type SessionMeta struct {
	ID           string    `json:"id"`
	Instruction  string    `json:"instruction"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

type SessionStore struct {
	baseDir string
	ttl     time.Duration
	locks   sync.Map // sessionID -> *sync.Mutex
	metas   sync.Map // sessionID -> *SessionMeta
	cancel  func()
}

func NewSessionStore(baseDir string, ttl time.Duration) (*SessionStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	s := &SessionStore{baseDir: baseDir, ttl: ttl}
	return s, nil
}

// StartCleanup starts a background goroutine that periodically removes expired sessions.
// Call the returned cancel func to stop it (e.g., on server shutdown).
func (s *SessionStore) StartCleanup(interval time.Duration) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.cleanExpired()
			case <-done:
				return
			}
		}
	}()
	cancel := func() { close(done) }
	s.cancel = cancel
	return cancel
}

func (s *SessionStore) getLock(sessionID string) *sync.Mutex {
	v, _ := s.locks.LoadOrStore(sessionID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func (s *SessionStore) jsonlPath(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID+".jsonl")
}

func (s *SessionStore) metaPath(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID+".meta.json")
}

func (s *SessionStore) Create(sessionID, instruction string) error {
	mu := s.getLock(sessionID)
	mu.Lock()
	defer mu.Unlock()

	meta := &SessionMeta{
		ID:           sessionID,
		Instruction:  instruction,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}

	// Write meta file
	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.metaPath(sessionID), metaData, 0644); err != nil {
		return err
	}

	// Create empty JSONL file
	f, err := os.Create(s.jsonlPath(sessionID))
	if err != nil {
		return err
	}
	f.Close()

	s.metas.Store(sessionID, meta)
	return nil
}

func (s *SessionStore) Exists(sessionID string) bool {
	_, ok := s.metas.Load(sessionID)
	if ok {
		return true
	}
	// Check disk
	_, err := os.Stat(s.metaPath(sessionID))
	return err == nil
}

func (s *SessionStore) GetMeta(sessionID string) (*SessionMeta, bool) {
	if v, ok := s.metas.Load(sessionID); ok {
		meta := v.(*SessionMeta)
		if time.Since(meta.LastActiveAt) > s.ttl {
			return nil, false
		}
		return meta, true
	}
	// Try loading from disk
	data, err := os.ReadFile(s.metaPath(sessionID))
	if err != nil {
		return nil, false
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, false
	}
	if time.Since(meta.LastActiveAt) > s.ttl {
		return nil, false
	}
	s.metas.Store(sessionID, &meta)
	return &meta, true
}

func (s *SessionStore) GetMessages(sessionID string) ([]*schema.Message, error) {
	mu := s.getLock(sessionID)
	mu.Lock()
	defer mu.Unlock()

	f, err := os.Open(s.jsonlPath(sessionID))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var msgs []*schema.Message
	scanner := bufio.NewScanner(f)
	// Increase buffer for large messages
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg schema.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			zap.S().Warnf("session %s: skip malformed message line: %v", sessionID, err)
			continue
		}
		msgs = append(msgs, &msg)
	}
	return msgs, scanner.Err()
}

func (s *SessionStore) Append(sessionID string, msgs ...*schema.Message) error {
	mu := s.getLock(sessionID)
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(s.jsonlPath(sessionID), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}

	// Update last active time
	s.touchMeta(sessionID)
	return nil
}

func (s *SessionStore) Delete(sessionID string) {
	mu := s.getLock(sessionID)
	mu.Lock()
	defer mu.Unlock()

	os.Remove(s.jsonlPath(sessionID))
	os.Remove(s.metaPath(sessionID))
	s.metas.Delete(sessionID)
	s.locks.Delete(sessionID)
}

func (s *SessionStore) touchMeta(sessionID string) {
	if v, ok := s.metas.Load(sessionID); ok {
		meta := v.(*SessionMeta)
		meta.LastActiveAt = time.Now()
		// Persist to disk
		data, _ := json.Marshal(meta)
		os.WriteFile(s.metaPath(sessionID), data, 0644)
	}
}

func (s *SessionStore) cleanExpired() {
	s.metas.Range(func(key, value any) bool {
		meta := value.(*SessionMeta)
		if time.Since(meta.LastActiveAt) > s.ttl {
			sid := key.(string)
			zap.S().Infof("cleaning expired session: %s", sid)
			s.Delete(sid)
		}
		return true
	})
}

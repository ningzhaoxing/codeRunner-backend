package checkpoint

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

type item struct {
	data      []byte
	updatedAt time.Time
}

// MemoryCheckPointStore implements core.CheckPointStore for Eino Runner.
// Used only for HITL interrupt/resume state persistence.
type MemoryCheckPointStore struct {
	ttl  time.Duration
	data sync.Map // checkPointID -> *item
	done chan struct{}
}

func NewMemoryCheckPointStore(ttl time.Duration) *MemoryCheckPointStore {
	return &MemoryCheckPointStore{
		ttl:  ttl,
		done: make(chan struct{}),
	}
}

func (s *MemoryCheckPointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	v, ok := s.data.Load(checkPointID)
	if !ok {
		return nil, false, nil
	}
	it := v.(*item)
	if time.Since(it.updatedAt) > s.ttl {
		s.data.Delete(checkPointID)
		return nil, false, nil
	}
	return it.data, true, nil
}

// Set stores checkpoint data. Method name is Set (not Put) per core.CheckPointStore interface.
func (s *MemoryCheckPointStore) Set(_ context.Context, checkPointID string, data []byte) error {
	s.data.Store(checkPointID, &item{data: data, updatedAt: time.Now()})
	return nil
}

func (s *MemoryCheckPointStore) Delete(checkPointID string) {
	s.data.Delete(checkPointID)
}

// StartCleanup starts periodic cleanup of expired entries.
func (s *MemoryCheckPointStore) StartCleanup(interval time.Duration) func() {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.data.Range(func(key, value any) bool {
					it := value.(*item)
					if time.Since(it.updatedAt) > s.ttl {
						zap.S().Infof("cleaning expired checkpoint: %s", key)
						s.data.Delete(key)
					}
					return true
				})
			case <-s.done:
				return
			}
		}
	}()
	return func() { close(s.done) }
}

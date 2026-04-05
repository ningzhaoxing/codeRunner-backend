package containerBasic

import (
	"context"
	"testing"
	"time"
)

func newTestPool(lang string, size int) *ContainerPool {
	images := map[string]string{lang: "test-image"}
	poolSizes := map[string]int{lang: size}
	return newContainerPool(nil, poolSizes, images)
}

func TestAcquireRelease(t *testing.T) {
	pool := newTestPool("golang", 2)
	defer pool.Close()

	ctx := context.Background()

	slot1, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("Acquire slot1 failed: %v", err)
	}
	slot2, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("Acquire slot2 failed: %v", err)
	}

	if slot1.Name == slot2.Name {
		t.Errorf("expected different slots, got same: %s", slot1.Name)
	}

	if slot1.HostPath == "" || slot2.HostPath == "" {
		t.Error("expected non-empty HostPath")
	}

	pool.Release("golang", slot1, true)
	slot3, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("Acquire after Release failed: %v", err)
	}
	if slot3.Name != slot1.Name {
		t.Errorf("expected to get back slot1 (%s), got %s", slot1.Name, slot3.Name)
	}
}

func TestAcquireTimeout(t *testing.T) {
	pool := newTestPool("golang", 1)
	defer pool.Close()

	ctx := context.Background()

	_, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_, err = pool.Acquire(timeoutCtx, "golang")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestAcquireUnsupportedLanguage(t *testing.T) {
	pool := newTestPool("golang", 1)
	defer pool.Close()

	_, err := pool.Acquire(context.Background(), "rust")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestAcquireAfterClose(t *testing.T) {
	pool := newTestPool("golang", 1)
	pool.Close()

	_, err := pool.Acquire(context.Background(), "golang")
	if err == nil {
		t.Fatal("expected error after Close")
	}
}

func TestReleaseAfterClose(t *testing.T) {
	pool := newTestPool("golang", 1)
	ctx := context.Background()

	slot, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	pool.Close()

	// Should not panic
	pool.Release("golang", slot, true)
}

func TestConcurrentAcquireRelease(t *testing.T) {
	pool := newTestPool("golang", 2)
	defer pool.Close()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			slot, err := pool.Acquire(ctx, "golang")
			if err != nil {
				done <- true
				return
			}
			time.Sleep(10 * time.Millisecond)
			pool.Release("golang", slot, true)
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

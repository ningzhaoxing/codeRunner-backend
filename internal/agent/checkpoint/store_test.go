package checkpoint

import (
	"context"
	"testing"
	"time"
)

func TestCheckPointStore_SetGet(t *testing.T) {
	store := NewMemoryCheckPointStore(time.Hour)
	ctx := context.Background()

	data := []byte("checkpoint-data")
	if err := store.Set(ctx, "sid-1", data); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.Get(ctx, "sid-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected checkpoint to exist")
	}
	if string(got) != string(data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestCheckPointStore_NotFound(t *testing.T) {
	store := NewMemoryCheckPointStore(time.Hour)
	_, ok, err := store.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected not found")
	}
}

func TestCheckPointStore_Expired(t *testing.T) {
	store := NewMemoryCheckPointStore(1 * time.Millisecond)
	store.Set(context.Background(), "sid-exp", []byte("data"))
	time.Sleep(5 * time.Millisecond)

	_, ok, _ := store.Get(context.Background(), "sid-exp")
	if ok {
		t.Fatal("expired checkpoint should not be returned")
	}
}

func TestCheckPointStore_Delete(t *testing.T) {
	store := NewMemoryCheckPointStore(time.Hour)
	store.Set(context.Background(), "sid-del", []byte("data"))
	store.Delete("sid-del")

	_, ok, _ := store.Get(context.Background(), "sid-del")
	if ok {
		t.Fatal("deleted checkpoint should not be returned")
	}
}

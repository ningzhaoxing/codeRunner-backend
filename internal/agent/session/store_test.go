package session

import (
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestSessionStore_CreateAndGetMessages(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStore(dir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	sid := "session-1"
	if err := store.Create(sid, "system prompt"); err != nil {
		t.Fatal(err)
	}

	if !store.Exists(sid) {
		t.Fatal("session should exist")
	}

	meta, ok := store.GetMeta(sid)
	if !ok {
		t.Fatal("meta should exist")
	}
	if meta.Instruction != "system prompt" {
		t.Fatalf("instruction = %q, want %q", meta.Instruction, "system prompt")
	}

	// Append messages
	err = store.Append(sid,
		schema.UserMessage("为什么 panic？"),
		schema.AssistantMessage("因为越界", nil),
	)
	if err != nil {
		t.Fatal(err)
	}

	msgs, err := store.GetMessages(sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].Content != "为什么 panic？" {
		t.Fatalf("msg[0].Content = %q", msgs[0].Content)
	}
}

func TestSessionStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSessionStore(dir, time.Hour)

	sid := "session-del"
	store.Create(sid, "test")
	store.Delete(sid)

	if store.Exists(sid) {
		t.Fatal("session should not exist after delete")
	}
}

func TestSessionStore_ExpiredSession(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSessionStore(dir, 1*time.Millisecond)

	sid := "session-exp"
	store.Create(sid, "test")
	time.Sleep(5 * time.Millisecond)

	_, ok := store.GetMeta(sid)
	if ok {
		t.Fatal("expired session should not be returned")
	}
}

package auth

import (
	"strings"
	"testing"
	"time"
)

func TestStateSigner_RoundTrip(t *testing.T) {
	signer := NewStateSigner([]byte("secret"), time.Minute)
	state, err := signer.Sign("/posts/1", time.Unix(1000, 0))
	if err != nil {
		t.Fatalf("Sign unexpected error: %v", err)
	}

	got, err := signer.Verify(state, time.Unix(1001, 0))
	if err != nil {
		t.Fatalf("Verify unexpected error: %v", err)
	}
	if got != "/posts/1" {
		t.Fatalf("return_to = %q, want /posts/1", got)
	}
}

func TestStateSigner_RejectsTamperedState(t *testing.T) {
	signer := NewStateSigner([]byte("secret"), time.Minute)
	state, err := signer.Sign("/posts/1", time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(state, ".", "x.", 1)

	if _, err := signer.Verify(tampered, time.Unix(1001, 0)); err == nil {
		t.Fatal("expected tampered state to fail")
	}
}

func TestStateSigner_RejectsExpiredState(t *testing.T) {
	signer := NewStateSigner([]byte("secret"), time.Minute)
	state, err := signer.Sign("/posts/1", time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := signer.Verify(state, time.Unix(1061, 0)); err == nil {
		t.Fatal("expected expired state to fail")
	}
}

func TestStateSigner_RejectsUnsafeReturnTo(t *testing.T) {
	signer := NewStateSigner([]byte("secret"), time.Minute)
	if _, err := signer.Sign("//evil.com", time.Unix(1000, 0)); err == nil {
		t.Fatal("expected unsafe return_to to fail")
	}
}

package auth

import (
	"testing"
	"time"
)

func TestJWTManager_SignAndParse(t *testing.T) {
	manager := NewJWTManager([]byte("secret"), time.Hour)
	user := User{
		ID:        "github:123",
		GitHubID:  123,
		Login:     "octocat",
		Name:      "The Octocat",
		AvatarURL: "https://avatars.githubusercontent.com/u/123?v=4",
	}

	token, err := manager.Sign(user, time.Unix(1000, 0))
	if err != nil {
		t.Fatalf("Sign unexpected error: %v", err)
	}
	got, err := manager.Parse(token, time.Unix(1001, 0))
	if err != nil {
		t.Fatalf("Parse unexpected error: %v", err)
	}
	if got.GitHubID != user.GitHubID || got.Login != user.Login || got.ID != "github:123" {
		t.Fatalf("parsed user = %+v, want %+v", got, user)
	}
}

func TestJWTManager_RejectsWrongSecret(t *testing.T) {
	manager := NewJWTManager([]byte("secret"), time.Hour)
	token, err := manager.Sign(User{ID: "github:123", GitHubID: 123, Login: "octocat"}, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}

	other := NewJWTManager([]byte("other"), time.Hour)
	if _, err := other.Parse(token, time.Unix(1001, 0)); err == nil {
		t.Fatal("expected wrong secret to fail")
	}
}

func TestJWTManager_RejectsExpiredToken(t *testing.T) {
	manager := NewJWTManager([]byte("secret"), time.Hour)
	token, err := manager.Sign(User{ID: "github:123", GitHubID: 123, Login: "octocat"}, time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Parse(token, time.Unix(4601, 0)); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

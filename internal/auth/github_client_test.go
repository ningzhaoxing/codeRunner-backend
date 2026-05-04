package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func TestGitHubClient_FetchUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token-123" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         123,
			"login":      "octocat",
			"name":       "The Octocat",
			"avatar_url": "https://avatars.githubusercontent.com/u/123?v=4",
		})
	}))
	defer server.Close()

	client := NewGitHubClient(oauth2.Config{}, server.URL, server.Client())
	user, err := client.FetchUser(context.Background(), "token-123")
	if err != nil {
		t.Fatalf("FetchUser unexpected error: %v", err)
	}
	if user.ID != "github:123" || user.GitHubID != 123 || user.Login != "octocat" {
		t.Fatalf("user = %+v", user)
	}
}

package auth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"golang.org/x/oauth2"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGitHubClient_FetchUser(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://api.test/user" {
				t.Fatalf("url = %q", req.URL.String())
			}
			if req.Header.Get("Authorization") != "Bearer token-123" {
				t.Fatalf("Authorization = %q", req.Header.Get("Authorization"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(bytes.NewBufferString(`{
					"id": 123,
					"login": "octocat",
					"name": "The Octocat",
					"avatar_url": "https://avatars.githubusercontent.com/u/123?v=4"
				}`)),
				Header: make(http.Header),
			}, nil
		}),
	}

	client := NewGitHubClient(oauth2.Config{}, "https://api.test", httpClient)
	user, err := client.FetchUser(context.Background(), "token-123")
	if err != nil {
		t.Fatalf("FetchUser unexpected error: %v", err)
	}
	if user.ID != "github:123" || user.GitHubID != 123 || user.Login != "octocat" {
		t.Fatalf("user = %+v", user)
	}
}

func TestGitHubClient_FetchUserRejectsNonSuccess(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	client := NewGitHubClient(oauth2.Config{}, "https://api.test", httpClient)
	if _, err := client.FetchUser(context.Background(), "token-123"); err == nil {
		t.Fatal("expected non-success response to fail")
	}
}

func TestGitHubClient_AuthCodeURLIncludesClientAndRedirect(t *testing.T) {
	client := NewGitHubClient(oauth2.Config{
		ClientID:    "client-id",
		RedirectURL: "http://example.com/auth/github/callback",
	}, "https://api.test", nil)

	url := client.AuthCodeURL("state-123")
	for _, want := range []string{
		"https://github.com/login/oauth/authorize",
		"client_id=client-id",
		"redirect_uri=http%3A%2F%2Fexample.com%2Fauth%2Fgithub%2Fcallback",
		"state=state-123",
	} {
		if !bytes.Contains([]byte(url), []byte(want)) {
			t.Fatalf("url = %q, missing %q", url, want)
		}
	}
}

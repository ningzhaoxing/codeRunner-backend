package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeGitHubClient struct {
	authURL string
	user    User
}

func (f fakeGitHubClient) AuthCodeURL(state string) string {
	return f.authURL + "?state=" + state
}

func (f fakeGitHubClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	return "token-123", nil
}

func (f fakeGitHubClient) FetchUser(ctx context.Context, accessToken string) (User, error) {
	return f.user, nil
}

func newTestAuthHandler() *Handler {
	cfg := Config{
		JWT:    JWTConfig{Secret: "secret", TTLSeconds: 604800, TTL: 7 * 24 * time.Hour},
		Cookie: CookieConfig{Name: "cr_auth", Secure: false},
	}.WithDefaults()
	service := NewService(cfg, fakeGitHubClient{
		authURL: "https://github.com/login/oauth/authorize",
		user:    User{ID: "github:123", GitHubID: 123, Login: "octocat", Name: "The Octocat", AvatarURL: "https://avatar"},
	}, func() time.Time { return time.Unix(1000, 0) })
	return NewHandler(service)
}

func TestHandler_LoginRedirectsToGitHub(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r, newTestAuthHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/github/login?return_to=/posts/1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://github.com/login/oauth/authorize") || !strings.Contains(loc, "state=") {
		t.Fatalf("Location = %q", loc)
	}
}

func TestHandler_MeUnauthorizedWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r, newTestAuthHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestHandler_LogoutClearsCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r, newTestAuthHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("Set-Cookie = %q", w.Header().Get("Set-Cookie"))
	}
}

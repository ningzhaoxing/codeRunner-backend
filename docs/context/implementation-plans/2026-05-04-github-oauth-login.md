# GitHub OAuth Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a minimal GitHub OAuth2 browser login layer with HttpOnly Cookie JWT, without changing Agent session ownership.

**Architecture:** Create an isolated `internal/auth` package for OAuth state, user JWT, GitHub API access, service orchestration, and Gin handlers. Extend existing YAML config loading, then register `/auth/*` routes from the existing HTTP router. Keep this separate from existing gRPC TokenIssuer auth to avoid mixing service-token and browser-user semantics.

**Tech Stack:** Go, Gin, `github.com/golang-jwt/jwt/v4`, `golang.org/x/oauth2`, standard `net/http`, existing Viper config.

---

## File Structure

- Create `internal/auth/config.go`: auth config structs and defaults.
- Create `internal/auth/return_to.go`: open-redirect-safe `return_to` normalization.
- Create `internal/auth/state.go`: signed OAuth state creation and validation.
- Create `internal/auth/jwt.go`: browser user JWT signing and parsing.
- Create `internal/auth/github_client.go`: GitHub token exchange and `/user` profile client.
- Create `internal/auth/service.go`: auth use-case orchestration.
- Create `internal/auth/handler.go`: Gin handlers and Cookie writing.
- Modify `internal/infrastructure/config/initConfig.go`: add `Auth auth.Config` to root config.
- Modify `configs/dev.yaml` and `configs/product.yaml`: add `auth` block.
- Modify `internal/interfaces/adapter/router/router.go`: register auth routes.
- Modify `go.mod` / `go.sum`: add `golang.org/x/oauth2`.

---

### Task 1: Config And Return Path Validation

**Files:**
- Create: `internal/auth/config.go`
- Create: `internal/auth/return_to.go`
- Test: `internal/auth/return_to_test.go`
- Modify: `internal/infrastructure/config/initConfig.go`
- Modify: `configs/dev.yaml`
- Modify: `configs/product.yaml`

- [ ] **Step 1: Write failing return_to tests**

Create `internal/auth/return_to_test.go`:

```go
package auth

import "testing"

func TestNormalizeReturnTo(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty defaults root", in: "", want: "/"},
		{name: "relative path accepted", in: "/posts/go-concurrency", want: "/posts/go-concurrency"},
		{name: "query accepted", in: "/posts?id=1", want: "/posts?id=1"},
		{name: "fragment accepted", in: "/posts#code", want: "/posts#code"},
		{name: "http rejected", in: "http://evil.com", wantErr: true},
		{name: "https rejected", in: "https://evil.com", wantErr: true},
		{name: "protocol relative rejected", in: "//evil.com", wantErr: true},
		{name: "non slash rejected", in: "posts/1", wantErr: true},
		{name: "backslash rejected", in: `/\evil`, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeReturnTo(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeReturnTo(%q) expected error, got nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeReturnTo(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeReturnTo(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestNormalizeReturnTo -count=1
```

Expected: FAIL with `undefined: NormalizeReturnTo`.

- [ ] **Step 3: Implement config and return_to**

Create `internal/auth/config.go`:

```go
package auth

import "time"

const (
	DefaultCookieName     = "cr_auth"
	DefaultJWTTTLSeconds  = 604800
	DefaultStateTTLSecond = 300
)

type Config struct {
	GitHub GitHubConfig `yaml:"github"`
	JWT    JWTConfig    `yaml:"jwt"`
	Cookie CookieConfig `yaml:"cookie"`
}

type GitHubConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
}

type JWTConfig struct {
	Secret     string        `yaml:"secret"`
	TTLSeconds int           `yaml:"ttl_seconds"`
	TTL        time.Duration `yaml:"-"`
}

type CookieConfig struct {
	Name   string `yaml:"name"`
	Secure bool   `yaml:"secure"`
}

func (c Config) WithDefaults() Config {
	if c.Cookie.Name == "" {
		c.Cookie.Name = DefaultCookieName
	}
	if c.JWT.TTLSeconds <= 0 {
		c.JWT.TTLSeconds = DefaultJWTTTLSeconds
	}
	c.JWT.TTL = time.Duration(c.JWT.TTLSeconds) * time.Second
	return c
}
```

Create `internal/auth/return_to.go`:

```go
package auth

import (
	"fmt"
	"net/url"
	"strings"
)

func NormalizeReturnTo(raw string) (string, error) {
	if raw == "" {
		return "/", nil
	}
	if strings.Contains(raw, `\`) {
		return "", fmt.Errorf("return_to contains backslash")
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "", fmt.Errorf("return_to must be a relative absolute path")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse return_to: %w", err)
	}
	if u.IsAbs() || u.Host != "" {
		return "", fmt.Errorf("return_to must not include scheme or host")
	}
	return raw, nil
}
```

Modify `internal/infrastructure/config/initConfig.go`:

```go
import (
	"codeRunner-siwu/internal/agent"
	"codeRunner-siwu/internal/auth"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Config struct {
	Server   ServerConfig      `yaml:"server"`
	Client   ClientConfig      `yaml:"client"`
	Logger   LoggerConfig      `yaml:"log"`
	Agent    agent.AgentConfig `yaml:"agent"`
	Auth     auth.Config       `yaml:"auth"`
	Mail     MailConfig        `yaml:"mail"`
	Feedback FeedbackConfig    `yaml:"feedback"`
}
```

In `LoadConfig`, after the existing `viper.Unmarshal` block succeeds and before `fmt.Println(config)`, assign:

```go
config.Auth = config.Auth.WithDefaults()
```

Add to `configs/dev.yaml`:

```yaml
auth:
  github:
    client_id: ${GITHUB_CLIENT_ID}
    client_secret: ${GITHUB_CLIENT_SECRET}
    redirect_url: ${GITHUB_REDIRECT_URL}
  jwt:
    secret: ${AUTH_JWT_SECRET}
    ttl_seconds: 604800
  cookie:
    name: "cr_auth"
    secure: false
```

Add to `configs/product.yaml`:

```yaml
auth:
  github:
    client_id: ${GITHUB_CLIENT_ID}
    client_secret: ${GITHUB_CLIENT_SECRET}
    redirect_url: ${GITHUB_REDIRECT_URL}
  jwt:
    secret: ${AUTH_JWT_SECRET}
    ttl_seconds: 604800
  cookie:
    name: "cr_auth"
    secure: true
```

- [ ] **Step 4: Run test to verify GREEN**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestNormalizeReturnTo -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/config.go internal/auth/return_to.go internal/auth/return_to_test.go internal/infrastructure/config/initConfig.go configs/dev.yaml configs/product.yaml
git commit -m "feat(auth): add oauth config and return path validation"
```

---

### Task 2: Signed OAuth State

**Files:**
- Create: `internal/auth/state.go`
- Test: `internal/auth/state_test.go`

- [ ] **Step 1: Write failing state tests**

Create `internal/auth/state_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestStateSigner -count=1
```

Expected: FAIL with `undefined: NewStateSigner`.

- [ ] **Step 3: Implement state signer**

Create `internal/auth/state.go`:

```go
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type StateSigner struct {
	secret []byte
	ttl    time.Duration
}

type statePayload struct {
	Nonce    string `json:"nonce"`
	ReturnTo string `json:"return_to"`
	Exp      int64  `json:"exp"`
}

func NewStateSigner(secret []byte, ttl time.Duration) *StateSigner {
	return &StateSigner{secret: secret, ttl: ttl}
}

func (s *StateSigner) Sign(returnTo string, now time.Time) (string, error) {
	normalized, err := NormalizeReturnTo(returnTo)
	if err != nil {
		return "", err
	}
	nonce, err := randomBase64URL(18)
	if err != nil {
		return "", err
	}
	payload := statePayload{
		Nonce:    nonce,
		ReturnTo: normalized,
		Exp:      now.Add(s.ttl).Unix(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(data)
	sig := s.sign(payloadPart)
	return payloadPart + "." + sig, nil
}

func (s *StateSigner) Verify(state string, now time.Time) (string, error) {
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid state format")
	}
	expectedSig := s.sign(parts[0])
	if !hmac.Equal([]byte(expectedSig), []byte(parts[1])) {
		return "", fmt.Errorf("invalid state signature")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode state payload: %w", err)
	}
	var payload statePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("parse state payload: %w", err)
	}
	if now.Unix() > payload.Exp {
		return "", fmt.Errorf("state expired")
	}
	return NormalizeReturnTo(payload.ReturnTo)
}

func (s *StateSigner) sign(payloadPart string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run test to verify GREEN**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestStateSigner -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/state.go internal/auth/state_test.go
git commit -m "feat(auth): add signed oauth state"
```

---

### Task 3: Browser User JWT

**Files:**
- Create: `internal/auth/jwt.go`
- Test: `internal/auth/jwt_test.go`

- [ ] **Step 1: Write failing JWT tests**

Create `internal/auth/jwt_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestJWTManager -count=1
```

Expected: FAIL with `undefined: NewJWTManager`.

- [ ] **Step 3: Implement JWT manager**

Create `internal/auth/jwt.go`:

```go
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type User struct {
	ID        string `json:"id"`
	GitHubID  int64  `json:"github_id"`
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

type userClaims struct {
	GitHubID  int64  `json:"github_id"`
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	jwt.RegisteredClaims
}

func NewJWTManager(secret []byte, ttl time.Duration) *JWTManager {
	return &JWTManager{secret: secret, ttl: ttl}
}

func (m *JWTManager) Sign(user User, now time.Time) (string, error) {
	if user.ID == "" {
		user.ID = fmt.Sprintf("github:%d", user.GitHubID)
	}
	claims := userClaims{
		GitHubID:  user.GitHubID,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *JWTManager) Parse(tokenString string, now time.Time) (User, error) {
	claims := &userClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(tk *jwt.Token) (interface{}, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	}, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		return User{}, err
	}
	if !token.Valid {
		return User{}, fmt.Errorf("token invalid")
	}
	return User{
		ID:        claims.Subject,
		GitHubID:  claims.GitHubID,
		Login:     claims.Login,
		Name:      claims.Name,
		AvatarURL: claims.AvatarURL,
	}, nil
}
```

- [ ] **Step 4: Run test to verify GREEN**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestJWTManager -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/jwt.go internal/auth/jwt_test.go go.mod go.sum
git commit -m "feat(auth): add browser user jwt"
```

---

### Task 4: GitHub OAuth Client

**Files:**
- Create: `internal/auth/github_client.go`
- Test: `internal/auth/github_client_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add OAuth dependency**

Run:

```bash
go get golang.org/x/oauth2
```

Expected: `go.mod` and `go.sum` include `golang.org/x/oauth2`.

- [ ] **Step 2: Write failing GitHub client tests**

Create `internal/auth/github_client_test.go`:

```go
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
```

- [ ] **Step 3: Run test to verify RED**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestGitHubClient -count=1
```

Expected: FAIL with `undefined: NewGitHubClient`.

- [ ] **Step 4: Implement GitHub client**

Create `internal/auth/github_client.go`:

```go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type GitHubOAuthClient interface {
	AuthCodeURL(state string) string
	ExchangeCode(ctx context.Context, code string) (string, error)
	FetchUser(ctx context.Context, accessToken string) (User, error)
}

type GitHubClient struct {
	oauthConfig oauth2.Config
	apiBaseURL  string
	httpClient  *http.Client
}

type githubUserResponse struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func NewGitHubClient(cfg oauth2.Config, apiBaseURL string, httpClient *http.Client) *GitHubClient {
	if cfg.Endpoint.AuthURL == "" {
		cfg.Endpoint = github.Endpoint
	}
	if apiBaseURL == "" {
		apiBaseURL = "https://api.github.com"
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &GitHubClient{oauthConfig: cfg, apiBaseURL: apiBaseURL, httpClient: httpClient}
}

func NewGitHubClientFromConfig(cfg Config) *GitHubClient {
	return NewGitHubClient(oauth2.Config{
		ClientID:     cfg.GitHub.ClientID,
		ClientSecret: cfg.GitHub.ClientSecret,
		RedirectURL:  cfg.GitHub.RedirectURL,
		Scopes:       []string{"read:user"},
		Endpoint:     github.Endpoint,
	}, "", http.DefaultClient)
}

func (c *GitHubClient) AuthCodeURL(state string) string {
	return c.oauthConfig.AuthCodeURL(state)
}

func (c *GitHubClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	tok, err := c.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func (c *GitHubClient) FetchUser(ctx context.Context, accessToken string) (User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+"/user", nil)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return User{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return User{}, fmt.Errorf("github user status %d", resp.StatusCode)
	}
	var gh githubUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return User{}, err
	}
	return User{
		ID:        fmt.Sprintf("github:%d", gh.ID),
		GitHubID:  gh.ID,
		Login:     gh.Login,
		Name:      gh.Name,
		AvatarURL: gh.AvatarURL,
	}, nil
}
```

- [ ] **Step 5: Run test to verify GREEN**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestGitHubClient -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/github_client.go internal/auth/github_client_test.go go.mod go.sum
git commit -m "feat(auth): add github oauth client"
```

---

### Task 5: Auth Service And Handlers

**Files:**
- Create: `internal/auth/service.go`
- Create: `internal/auth/handler.go`
- Test: `internal/auth/handler_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `internal/auth/handler_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify RED**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestHandler -count=1
```

Expected: FAIL with `undefined: Handler` or `undefined: NewService`.

- [ ] **Step 3: Implement service**

Create `internal/auth/service.go`:

```go
package auth

import (
	"context"
	"fmt"
	"time"
)

type Clock func() time.Time

type Service struct {
	cfg          Config
	githubClient GitHubOAuthClient
	stateSigner  *StateSigner
	jwtManager   *JWTManager
	now          Clock
}

func NewService(cfg Config, githubClient GitHubOAuthClient, now Clock) *Service {
	cfg = cfg.WithDefaults()
	if now == nil {
		now = time.Now
	}
	return &Service{
		cfg:          cfg,
		githubClient: githubClient,
		stateSigner:  NewStateSigner([]byte(cfg.JWT.Secret), 5*time.Minute),
		jwtManager:   NewJWTManager([]byte(cfg.JWT.Secret), cfg.JWT.TTL),
		now:          now,
	}
}

func (s *Service) LoginURL(returnTo string) (string, error) {
	normalized, err := NormalizeReturnTo(returnTo)
	if err != nil {
		return "", err
	}
	state, err := s.stateSigner.Sign(normalized, s.now())
	if err != nil {
		return "", err
	}
	return s.githubClient.AuthCodeURL(state), nil
}

func (s *Service) Callback(ctx context.Context, code, state string) (User, string, string, error) {
	if code == "" || state == "" {
		return User{}, "", "", fmt.Errorf("code and state are required")
	}
	returnTo, err := s.stateSigner.Verify(state, s.now())
	if err != nil {
		return User{}, "", "", err
	}
	accessToken, err := s.githubClient.ExchangeCode(ctx, code)
	if err != nil {
		return User{}, "", "", fmt.Errorf("exchange github code: %w", err)
	}
	user, err := s.githubClient.FetchUser(ctx, accessToken)
	if err != nil {
		return User{}, "", "", fmt.Errorf("fetch github user: %w", err)
	}
	token, err := s.jwtManager.Sign(user, s.now())
	if err != nil {
		return User{}, "", "", fmt.Errorf("sign jwt: %w", err)
	}
	return user, token, returnTo, nil
}

func (s *Service) ParseUser(token string) (User, error) {
	return s.jwtManager.Parse(token, s.now())
}

func (s *Service) CookieName() string {
	return s.cfg.Cookie.Name
}

func (s *Service) CookieSecure() bool {
	return s.cfg.Cookie.Secure
}

func (s *Service) CookieMaxAge() int {
	return int(s.cfg.JWT.TTL.Seconds())
}
```

- [ ] **Step 4: Implement handler**

Create `internal/auth/handler.go`:

```go
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(r gin.IRoutes, h *Handler) {
	r.GET("/auth/github/login", h.Login)
	r.GET("/auth/github/callback", h.Callback)
	r.GET("/auth/me", h.Me)
	r.POST("/auth/logout", h.Logout)
}

func (h *Handler) Login(c *gin.Context) {
	url, err := h.service.LoginURL(c.Query("return_to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid return_to"})
		return
	}
	c.Redirect(http.StatusFound, url)
}

func (h *Handler) Callback(c *gin.Context) {
	_, token, returnTo, err := h.service.Callback(c.Request.Context(), c.Query("code"), c.Query("state"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"message": "github oauth callback failed"})
		return
	}
	h.setCookie(c, token, h.service.CookieMaxAge())
	c.Redirect(http.StatusFound, returnTo)
}

func (h *Handler) Me(c *gin.Context) {
	token, err := c.Cookie(h.service.CookieName())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}
	user, err := h.service.ParseUser(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) Logout(c *gin.Context) {
	h.setCookie(c, "", 0)
	c.Status(http.StatusNoContent)
}

func (h *Handler) setCookie(c *gin.Context, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		h.service.CookieName(),
		value,
		maxAge,
		"/",
		"",
		h.service.CookieSecure(),
		true,
	)
}
```

- [ ] **Step 5: Run handler tests**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestHandler -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/service.go internal/auth/handler.go internal/auth/handler_test.go
git commit -m "feat(auth): add github oauth handlers"
```

---

### Task 6: Route Registration And Server Wiring

**Files:**
- Modify: `internal/interfaces/adapter/router/router.go`
- Modify: `internal/interfaces/adapter/initialize/app.go`
- Test: `internal/auth/handler_test.go`

- [ ] **Step 1: Write route registration expectation**

Append to `internal/auth/handler_test.go`:

```go
func TestRegisterRoutes_RegistersAuthPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r, newTestAuthHandler())

	paths := map[string]bool{}
	for _, route := range r.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /auth/github/login",
		"GET /auth/github/callback",
		"GET /auth/me",
		"POST /auth/logout",
	} {
		if !paths[want] {
			t.Fatalf("missing route %s; routes=%v", want, paths)
		}
	}
}
```

- [ ] **Step 2: Run auth route test**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -run TestRegisterRoutes -count=1
```

Expected: PASS once `RegisterRoutes` exists from Task 5.

- [ ] **Step 3: Wire auth into main router**

Modify `internal/interfaces/adapter/router/router.go`:

```go
import (
	"codeRunner-siwu/internal/agent"
	"codeRunner-siwu/internal/auth"
	serverService "codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/interfaces/controller"
	ctrlFeedback "codeRunner-siwu/internal/interfaces/controller/feedback"
	serverHandler "codeRunner-siwu/internal/interfaces/controller/server"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func ApiRouter(r *gin.Engine, svc serverService.ServerService, authHandler *auth.Handler) {
	r.GET("/ws", controller.APIs.CodeRunnerSrv.HandleServer())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.POST("/execute", serverHandler.ExecuteHandler(svc, 30*time.Second))
	r.POST("/api/feedback", ctrlFeedback.HandleFeedback(controller.APIs.FeedbackSvc))
	if authHandler != nil {
		auth.RegisterRoutes(r, authHandler)
	}
}
```

Modify `internal/interfaces/adapter/initialize/app.go`:

```go
authSvc := auth.NewService(c.Auth, auth.NewGitHubClientFromConfig(c.Auth), time.Now)
authHandler := auth.NewHandler(authSvc)
router.ApiRouter(r, serverSvc, authHandler)
```

Add imports:

```go
"codeRunner-siwu/internal/auth"
```

Keep `router.AgentRouter(r, agentSvc)` unchanged.

- [ ] **Step 4: Run compile-focused tests**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth ./internal/interfaces/adapter/router ./internal/interfaces/adapter/initialize -count=1
```

Expected: PASS or `[no test files]` for router/initialize packages.

- [ ] **Step 5: Commit**

```bash
git add internal/interfaces/adapter/router/router.go internal/interfaces/adapter/initialize/app.go internal/auth/handler_test.go
git commit -m "feat(auth): register oauth routes"
```

---

### Task 7: Final Verification

**Files:**
- No new files.

- [ ] **Step 1: Run auth package tests**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./internal/auth -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full suite**

Run:

```bash
env GOCACHE=/Users/ningzhaoxing/Desktop/code_runner/codeRunner-backend/.gocache go test ./... -count=1
```

Expected in sandbox: packages that bind local ports may fail with `operation not permitted`. If so, rerun the same command with escalated permissions. Expected outside sandbox: PASS.

- [ ] **Step 3: Manual smoke commands**

Start server with env vars:

```bash
APP_MODE=server \
GITHUB_CLIENT_ID=dummy \
GITHUB_CLIENT_SECRET=dummy \
GITHUB_REDIRECT_URL=http://localhost:7979/auth/github/callback \
AUTH_JWT_SECRET=dev-secret \
JWT_SECRET=grpc-secret \
AUTH_PASSWORD=grpc-password \
go run cmd/api/main.go
```

Verify login redirect:

```bash
curl -i 'http://localhost:7979/auth/github/login?return_to=/posts/1'
```

Expected: `302 Found` with `Location` starting with `https://github.com/login/oauth/authorize?`.

Verify unauthenticated me:

```bash
curl -i 'http://localhost:7979/auth/me'
```

Expected: `401 Unauthorized`.

- [ ] **Step 4: Commit any verification-only doc updates if needed**

If README or `docs/agent-api.md` is updated with auth endpoint notes:

```bash
git add README.md docs/agent-api.md
git commit -m "docs(auth): document github oauth endpoints"
```

If no docs are changed, skip this commit.

---

## Self-Review Checklist

- Design goal covered by Tasks 1-6.
- No database, Redis, Agent session ownership, claim, quota, or frontend work included.
- State is signed and 5-minute TTL.
- Cookie is HttpOnly, `SameSite=Lax`, `Path=/`, 7-day max age.
- Config uses `configs/*.yaml` with env expansion.
- Existing gRPC TokenIssuer remains separate.

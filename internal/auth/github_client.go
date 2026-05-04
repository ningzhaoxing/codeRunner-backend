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

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

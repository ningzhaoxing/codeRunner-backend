package auth

import "time"

const (
	DefaultCookieName     = "cr_auth"
	DefaultJWTTTLSeconds  = 604800
	DefaultStateTTLSecond = 300
)

type Config struct {
	GitHub          GitHubConfig `yaml:"github"`
	JWT             JWTConfig    `yaml:"jwt"`
	Cookie          CookieConfig `yaml:"cookie"`
	FrontendBaseURL string       `yaml:"frontend_base_url"`
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

func (c Config) Validate() error {
	if c.GitHub.ClientID == "" {
		return ErrMissingGitHubClientID
	}
	if c.GitHub.ClientSecret == "" {
		return ErrMissingGitHubClientSecret
	}
	if c.GitHub.RedirectURL == "" {
		return ErrMissingGitHubRedirectURL
	}
	if c.JWT.Secret == "" {
		return ErrMissingJWTSecret
	}
	return nil
}

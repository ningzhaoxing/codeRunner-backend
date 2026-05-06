package auth

import "errors"

var (
	ErrMissingGitHubClientID     = errors.New("missing github client id")
	ErrMissingGitHubClientSecret = errors.New("missing github client secret")
	ErrMissingGitHubRedirectURL  = errors.New("missing github redirect url")
	ErrMissingJWTSecret          = errors.New("missing auth jwt secret")
)

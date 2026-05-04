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

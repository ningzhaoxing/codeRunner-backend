package agent

import (
	"testing"
)

func TestNewAgentService_DisabledReturnsError(t *testing.T) {
	cfg := DefaultAgentConfig()
	cfg.Enabled = false
	_, err := NewAgentService(nil, cfg, t.TempDir())
	if err == nil {
		t.Fatal("expected error when agent disabled")
	}
}

func TestNewAgentService_MissingAPIKeyReturnsError(t *testing.T) {
	cfg := DefaultAgentConfig()
	cfg.Enabled = true
	cfg.Provider = "claude"
	cfg.Claude.APIKey = "" // missing
	_, err := NewAgentService(nil, cfg, t.TempDir())
	if err == nil {
		t.Fatal("expected error when Claude API key missing")
	}
}

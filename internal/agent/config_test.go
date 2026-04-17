package agent

import (
	"testing"
	"time"
)

func TestAgentConfig_Defaults(t *testing.T) {
	cfg := DefaultAgentConfig()
	if cfg.MaxSteps != 10 {
		t.Fatalf("MaxSteps = %d, want 10", cfg.MaxSteps)
	}
	if cfg.GetSessionTTL() != time.Hour {
		t.Fatalf("SessionTTL = %v, want 1h", cfg.GetSessionTTL())
	}
	if cfg.GetProposalTTL() != 10*time.Minute {
		t.Fatalf("ProposalTTL = %v, want 10m", cfg.GetProposalTTL())
	}
}

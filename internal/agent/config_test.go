package agent

import (
	"reflect"
	"testing"
	"time"

	"codeRunner-siwu/internal/agent/ai"
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

func TestAgentConfig_ToAIConfigIncludesProviderEndpoints(t *testing.T) {
	cfg := DefaultAgentConfig()
	cfg.Provider = "qwen"
	cfg.Claude.APIKey = "claude-key"
	cfg.Claude.Model = "claude-model"
	cfg.Claude.BaseURL = "https://claude.example/v1"
	cfg.OpenAI.APIKey = "openai-key"
	cfg.OpenAI.Model = "openai-model"
	cfg.OpenAI.BaseURL = "https://openai.example/v1"
	cfg.Qwen.APIKey = "qwen-key"
	cfg.Qwen.Model = "qwen-model"
	cfg.Qwen.BaseURL = "https://qwen.example/v1"

	got := cfg.ToAIConfig()
	want := ai.Config{Provider: "qwen"}
	want.Claude.APIKey = "claude-key"
	want.Claude.Model = "claude-model"
	want.Claude.BaseURL = "https://claude.example/v1"
	want.OpenAI.APIKey = "openai-key"
	want.OpenAI.Model = "openai-model"
	want.OpenAI.BaseURL = "https://openai.example/v1"
	want.Qwen.APIKey = "qwen-key"
	want.Qwen.Model = "qwen-model"
	want.Qwen.BaseURL = "https://qwen.example/v1"

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToAIConfig() = %#v, want %#v", got, want)
	}
}

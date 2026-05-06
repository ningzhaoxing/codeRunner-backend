package agent

import (
	"time"

	"codeRunner-siwu/internal/agent/ai"
)

type AgentConfig struct {
	Enabled       bool   `yaml:"enabled"`
	APIKey        string `yaml:"api_key"`
	Provider      string `yaml:"provider"`
	MaxSteps      int    `yaml:"max_steps"`
	SessionTTL    int    `yaml:"session_ttl"`
	ProposalTTL   int    `yaml:"proposal_ttl"`
	Summarization struct {
		TriggerTokens int `yaml:"trigger_tokens"`
	} `yaml:"summarization"`
	Reduction struct {
		MaxLengthForTrunc int `yaml:"max_length_for_trunc"`
		MaxTokensForClear int `yaml:"max_tokens_for_clear"`
	} `yaml:"reduction"`
	Claude struct {
		APIKey  string `yaml:"api_key"`
		Model   string `yaml:"model"`
		BaseURL string `yaml:"base_url"`
	} `yaml:"claude"`
	OpenAI struct {
		APIKey  string `yaml:"api_key"`
		Model   string `yaml:"model"`
		BaseURL string `yaml:"base_url"`
	} `yaml:"openai"`
	Qwen struct {
		APIKey  string `yaml:"api_key"`
		Model   string `yaml:"model"`
		BaseURL string `yaml:"base_url"`
	} `yaml:"qwen"`
}

func (c *AgentConfig) GetSessionTTL() time.Duration {
	if c.SessionTTL <= 0 {
		return time.Hour
	}
	return time.Duration(c.SessionTTL) * time.Second
}

func (c *AgentConfig) GetProposalTTL() time.Duration {
	if c.ProposalTTL <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(c.ProposalTTL) * time.Second
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Enabled:     false,
		Provider:    "claude",
		MaxSteps:    10,
		SessionTTL:  3600,
		ProposalTTL: 600,
	}
}

func (c AgentConfig) ToAIConfig() ai.Config {
	cfg := ai.Config{Provider: c.Provider}
	cfg.Claude.APIKey = c.Claude.APIKey
	cfg.Claude.Model = c.Claude.Model
	cfg.Claude.BaseURL = c.Claude.BaseURL
	cfg.OpenAI.APIKey = c.OpenAI.APIKey
	cfg.OpenAI.Model = c.OpenAI.Model
	cfg.OpenAI.BaseURL = c.OpenAI.BaseURL
	cfg.Qwen.APIKey = c.Qwen.APIKey
	cfg.Qwen.Model = c.Qwen.Model
	cfg.Qwen.BaseURL = c.Qwen.BaseURL
	return cfg
}

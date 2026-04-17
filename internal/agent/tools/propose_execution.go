package tools

import (
	"context"
	"encoding/gob"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

func init() {
	gob.Register(&ProposalInfo{})
}

// ProposalInfo is the interrupt data sent to the frontend and persisted as state.
type ProposalInfo struct {
	ProposalID  string `json:"proposal_id"`
	Code        string `json:"code"`
	Language    string `json:"language"`
	Description string `json:"description"`
}

type proposeExecutionInput struct {
	NewCode     string `json:"new_code"`
	Language    string `json:"language"`
	Description string `json:"description"`
}

type ProposeExecutionTool struct{}

func NewProposeExecutionTool() *ProposeExecutionTool {
	return &ProposeExecutionTool{}
}

func (t *ProposeExecutionTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "propose_execution",
		Desc: "Propose code for execution. Use when you have a concrete, complete code change ready to run. NOT for speculative or incomplete code.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"new_code": {
				Type:     schema.String,
				Desc:     "The complete code to execute",
				Required: true,
			},
			"language": {
				Type:     schema.String,
				Desc:     "Programming language. Must be one of: golang, python, javascript, java, c",
				Required: true,
			},
			"description": {
				Type:     schema.String,
				Desc:     "Brief explanation of what this code does or what changes were made",
				Required: true,
			},
		}),
	}, nil
}

func (t *ProposeExecutionTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	// Check if we're resuming from an interrupt
	wasInterrupted, hasState, state := tool.GetInterruptState[*ProposalInfo](ctx)
	if wasInterrupted && hasState {
		// Resumed — the execution result should be available via GetResumeContext
		isTarget, hasData, data := tool.GetResumeContext[string](ctx)
		if isTarget && hasData {
			return data, nil // Return execution result to LLM
		}
		// Not the resume target or no data — re-interrupt with same state
		return "", tool.StatefulInterrupt(ctx, state, state)
	}

	// First call — parse input and interrupt
	var input proposeExecutionInput
	if err := sonic.UnmarshalString(argumentsInJSON, &input); err != nil {
		return "", fmt.Errorf("parse propose_execution input: %w", err)
	}

	normalized, err := normalizeLanguage(input.Language)
	if err != nil {
		return fmt.Sprintf("Error: %s. Supported languages: golang, python, javascript, java, c", err.Error()), nil
	}

	proposalInfo := &ProposalInfo{
		ProposalID:  uuid.NewString(),
		Code:        input.NewCode,
		Language:    normalized,
		Description: input.Description,
	}

	// info = user-facing interrupt data (appears in AgentEvent, frontend displays it)
	// state = internal state (gob-serialized, restored via GetInterruptState on resume)
	return "", tool.StatefulInterrupt(ctx, proposalInfo, proposalInfo)
}

// IsInvokable marks this tool as invokable (not streamable).
func (t *ProposeExecutionTool) IsInvokable() bool {
	return true
}

var languageMap = map[string]string{
	"go": "golang", "Go": "golang", "golang": "golang",
	"python": "python", "Python": "python", "py": "python",
	"javascript": "javascript", "js": "javascript", "JavaScript": "javascript",
	"java": "java", "Java": "java",
	"c": "c", "C": "c",
}

func normalizeLanguage(lang string) (string, error) {
	lang = strings.TrimSpace(lang)
	if normalized, ok := languageMap[lang]; ok {
		return normalized, nil
	}
	return "", fmt.Errorf("unsupported language: %q", lang)
}

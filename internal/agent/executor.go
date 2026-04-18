package agent

import (
	"context"
	"fmt"
	"time"

	"codeRunner-siwu/api/proto"
	serverService "codeRunner-siwu/internal/application/service/server"
)

// CodeExecutor runs code synchronously and returns the result.
type CodeExecutor interface {
	Execute(ctx context.Context, req ExecuteRequest) (ExecResult, error)
}

type ExecuteRequest struct {
	ProposalID string
	Code       string
	Language   string
}

type ExecResult struct {
	Result string
	Err    string
}

type codeRunnerExecutor struct {
	serverService serverService.ServerService
	timeout       time.Duration
}

func NewCodeExecutor(svc serverService.ServerService, timeout time.Duration) CodeExecutor {
	return &codeRunnerExecutor{serverService: svc, timeout: timeout}
}

func (e *codeRunnerExecutor) Execute(ctx context.Context, req ExecuteRequest) (ExecResult, error) {
	resp, err := e.serverService.ExecuteSync(ctx, &proto.ExecuteRequest{
		Id:        req.ProposalID,
		Uid:       0,
		Language:  req.Language,
		CodeBlock: req.Code,
	}, e.timeout)
	if err != nil {
		return ExecResult{}, fmt.Errorf("execute sync: %w", err)
	}
	return ExecResult{Result: resp.Result, Err: resp.Err}, nil
}

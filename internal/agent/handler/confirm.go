package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"codeRunner-siwu/internal/agent"
	"codeRunner-siwu/internal/agent/tools"
	"codeRunner-siwu/internal/infrastructure/metrics"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

type confirmRequest struct {
	SessionID  string `json:"session_id"`
	ProposalID string `json:"proposal_id"`
}

// ConfirmHandler executes the proposed code and resumes the agent with the result.
func ConfirmHandler(svc *agent.AgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		status := "success"
		var req confirmRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
			return
		}
		if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.ProposalID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "session_id and proposal_id are required"})
			return
		}

		// Validate session exists
		if _, ok := svc.SessionStore.GetMeta(req.SessionID); !ok {
			c.JSON(http.StatusNotFound, gin.H{"message": "session not found or expired"})
			return
		}

		// Retrieve the interrupt ID stored by the chat handler
		interruptIDVal, ok := svc.InterruptIDs.Load(req.SessionID)
		if !ok {
			c.JSON(http.StatusConflict, gin.H{"message": "no pending interrupt for this session"})
			return
		}
		interruptID := interruptIDVal.(string)

		// Retrieve proposal info stored by the chat handler
		proposalVal, ok := svc.Proposals.Load(req.SessionID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"message": "proposal not found"})
			return
		}
		proposal := proposalVal.(*tools.ProposalInfo)

		// Validate the proposal ID matches
		if proposal.ProposalID != req.ProposalID {
			c.JSON(http.StatusConflict, gin.H{"message": "proposal_id does not match pending proposal"})
			return
		}

		// Set SSE headers before any writes
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		// Notify client that execution is starting
		sseEvent(c, "message", ssePayload{Content: "executing"})

		ctx, cancel := context.WithCancel(c.Request.Context())
		svc.Cancels.Store(req.SessionID, cancel)
		defer func() {
			svc.Cancels.Delete(req.SessionID)
			cancel()
		}()

		// Execute the code synchronously
		result, execErr := svc.Executor.Execute(ctx, agent.ExecuteRequest{
			ProposalID: proposal.ProposalID,
			Code:       proposal.Code,
			Language:   proposal.Language,
		})

		var resultStr string
		if execErr != nil {
			resultStr = fmt.Sprintf("Execution failed: %s", execErr.Error())
		} else {
			var sb strings.Builder
			sb.WriteString("Execution result:\n")
			if result.Result != "" {
				sb.WriteString("stdout: ")
				sb.WriteString(result.Result)
				sb.WriteString("\n")
			}
			if result.Err != "" {
				sb.WriteString("stderr: ")
				sb.WriteString(result.Err)
				sb.WriteString("\n")
			}
			if result.Result == "" && result.Err == "" {
				sb.WriteString("(no output)\n")
			}
			resultStr = sb.String()
		}

		// Clear interrupt state — session is being resumed
		svc.InterruptIDs.Delete(req.SessionID)
		svc.Proposals.Delete(req.SessionID)

		// Resume the agent, passing the execution result to the interrupted tool
		iter, resumeErr := svc.Runner.ResumeWithParams(ctx, req.SessionID, &adk.ResumeParams{
			Targets: map[string]any{
				interruptID: resultStr,
			},
		})
		if resumeErr != nil {
			status = "error"
			sseEvent(c, "error", ssePayload{Error: resumeErr.Error()})
			metrics.AgentChatDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
			return
		}

		// Persist the propose_execution call + result so the next turn's history
		// reflects the actual tool exchange. Use the proposal_id as a stable
		// tool_call_id since Eino does not surface the original LLM tool_call_id here.
		assistantToolCall := &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   proposal.ProposalID,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "propose_execution",
					Arguments: fmt.Sprintf(`{"language":%q,"description":%q,"new_code":%q}`, proposal.Language, proposal.Description, proposal.Code),
				},
			}},
		}
		toolResult := schema.ToolMessage(resultStr, proposal.ProposalID, schema.WithToolName("propose_execution"))
		_ = svc.SessionStore.Append(req.SessionID, assistantToolCall, toolResult)

		var produced []*schema.Message
		var streamingChunks []*schema.Message

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}

			if event.Err != nil {
				status = "error"
				sseEvent(c, "error", ssePayload{Error: event.Err.Error()})
				break
			}

			// Capture any new interrupt ID if the agent re-interrupts
			if event.Action != nil && event.Action.Interrupted != nil {
				for _, ic := range event.Action.Interrupted.InterruptContexts {
					if ic != nil && ic.IsRootCause {
						svc.InterruptIDs.Store(req.SessionID, ic.ID)
						if p, ok := ic.Info.(*tools.ProposalInfo); ok {
							svc.Proposals.Store(req.SessionID, p)
							sseEvent(c, "interrupt", ssePayload{Proposal: p})
						}
						break
					}
				}
			}

			if event.Output != nil && event.Output.MessageOutput != nil {
				mv := event.Output.MessageOutput
				if mv.IsStreaming && mv.MessageStream != nil {
					stream := mv.MessageStream
					streamingChunks = streamingChunks[:0]
					for {
						chunk, recvErr := stream.Recv()
						if errors.Is(recvErr, io.EOF) {
							break
						}
						if recvErr != nil {
							break
						}
						if chunk == nil {
							continue
						}
						streamingChunks = append(streamingChunks, chunk)
						sseEvent(c, "stream_chunk", ssePayload{Content: chunk.Content})
					}
					if len(streamingChunks) > 0 {
						if full, err := schema.ConcatMessages(streamingChunks); err == nil && full != nil {
							produced = append(produced, full)
						}
					}
				} else if mv.Message != nil {
					produced = append(produced, mv.Message)
					eventType := "message"
					if mv.Message.Role == schema.Tool {
						eventType = "tool_result"
					}
					sseEvent(c, eventType, ssePayload{Content: mv.Message.Content})
				}
			}
		}

		if len(produced) > 0 {
			_ = svc.SessionStore.Append(req.SessionID, produced...)
		}

		metrics.AgentChatDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
		sseEvent(c, "done", ssePayload{})
	}
}

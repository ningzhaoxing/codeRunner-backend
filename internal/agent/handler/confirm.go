package handler

import (
	"encoding/json"
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
		execPayload, _ := json.Marshal(gin.H{"type": "executing"})
		sseData(c, string(execPayload))

		ctx := c.Request.Context()

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
			errPayload, _ := json.Marshal(gin.H{"error": resumeErr.Error()})
			sseEvent(c, "error", string(errPayload))
			metrics.AgentChatDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
			return
		}

		var assistantContent strings.Builder

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}

			if event.Err != nil {
				status = "error"
				errPayload, _ := json.Marshal(gin.H{"error": event.Err.Error()})
				sseEvent(c, "error", string(errPayload))
				break
			}

			// Capture any new interrupt ID if the agent re-interrupts
			if event.Action != nil && event.Action.Interrupted != nil {
				for _, ic := range event.Action.Interrupted.InterruptContexts {
					if ic != nil && ic.IsRootCause {
						svc.InterruptIDs.Store(req.SessionID, ic.ID)
						if p, ok := ic.Info.(*tools.ProposalInfo); ok {
							svc.Proposals.Store(req.SessionID, p)
							payload, _ := json.Marshal(gin.H{
								"type":     "proposal",
								"proposal": p,
							})
							sseEvent(c, "interrupt", string(payload))
						}
						break
					}
				}
			}

			if event.Output != nil && event.Output.MessageOutput != nil {
				mv := event.Output.MessageOutput
				if mv.IsStreaming && mv.MessageStream != nil {
					for {
						chunk, recvErr := mv.MessageStream.Recv()
						if errors.Is(recvErr, io.EOF) {
							break
						}
						if recvErr != nil {
							break
						}
						if chunk == nil || chunk.Content == "" {
							continue
						}
						if chunk.Role == schema.Assistant {
							assistantContent.WriteString(chunk.Content)
						}
						payload, _ := json.Marshal(gin.H{
							"type":    "content",
							"role":    chunk.Role,
							"content": chunk.Content,
						})
						sseData(c, string(payload))
					}
				} else if mv.Message != nil && mv.Message.Content != "" {
					if mv.Message.Role == schema.Assistant {
						assistantContent.WriteString(mv.Message.Content)
					}
					payload, _ := json.Marshal(gin.H{
						"type":    "content",
						"role":    mv.Message.Role,
						"content": mv.Message.Content,
					})
					sseData(c, string(payload))
				}
			}
		}

		// Persist assistant response
		if assistantContent.Len() > 0 {
			_ = svc.SessionStore.Append(req.SessionID, schema.AssistantMessage(assistantContent.String(), nil))
		}

		metrics.AgentChatDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
		sseEvent(c, "done", "{}")
	}
}

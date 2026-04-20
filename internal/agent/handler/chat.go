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
	"github.com/google/uuid"
)

// --- request types ---

type codeBlock struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

type articleCtx struct {
	ArticleID      string      `json:"article_id"`
	ArticleContent string      `json:"article_content"`
	CodeBlocks     []codeBlock `json:"code_blocks"`
}

type chatRequest struct {
	SessionID   string      `json:"session_id"`
	UserMessage string      `json:"user_message"`
	ArticleCtx  *articleCtx `json:"article_ctx"`
}

// --- SSE helpers ---

type ssePayload struct {
	Type      string      `json:"type"`
	Content   string      `json:"content,omitempty"`
	ToolCalls interface{} `json:"tool_calls,omitempty"`
	Error     string      `json:"error,omitempty"`
	Proposal  interface{} `json:"proposal,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
}

func sseEvent(c *gin.Context, event string, payload ssePayload) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, string(data))
	c.Writer.Flush()
}

func sseKeepAlive(c *gin.Context) {
	fmt.Fprintf(c.Writer, ": keepalive\n\n")
	c.Writer.Flush()
}

// --- instruction builder ---

func buildInstruction(ctx *articleCtx) string {
	if ctx == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("You are a helpful coding assistant for a blog platform.\n\n")
	if ctx.ArticleContent != "" {
		sb.WriteString("## Article Context\n")
		sb.WriteString(ctx.ArticleContent)
		sb.WriteString("\n\n")
	}
	if len(ctx.CodeBlocks) > 0 {
		sb.WriteString("## Code Blocks in Article\n")
		for i, cb := range ctx.CodeBlocks {
			sb.WriteString(fmt.Sprintf("### Block %d (%s)\n```%s\n%s\n```\n\n", i+1, cb.Language, cb.Language, cb.Code))
		}
	}
	sb.WriteString("## Instructions\n")
	sb.WriteString("- Answer questions about the article and code blocks above.\n")
	sb.WriteString("- You can run code using the available code execution tools.\n")
	sb.WriteString("- Be concise and helpful.\n")
	return sb.String()
}

// --- handler ---

func ChatHandler(svc *agent.AgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		status := "success"

		var req chatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
			return
		}

		if strings.TrimSpace(req.UserMessage) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "user_message is required"})
			return
		}

		// Determine mode
		hasSession := req.SessionID != ""
		hasArticle := req.ArticleCtx != nil

		if !hasSession && !hasArticle {
			c.JSON(http.StatusBadRequest, gin.H{"message": "either session_id or article_ctx is required"})
			return
		}

		var sessionID string
		var isNew bool
		var instruction string

		switch {
		case !hasSession && hasArticle:
			// create mode
			sessionID = uuid.New().String()
			isNew = true
			instruction = buildInstruction(req.ArticleCtx)
			if err := svc.SessionStore.Create(sessionID, instruction); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to create session"})
				return
			}
			metrics.AgentSessionsActive.Inc()

		case hasSession && hasArticle:
			// reset mode — delete old session, create fresh one with same ID
			sessionID = req.SessionID
			svc.SessionStore.Delete(sessionID)
			instruction = buildInstruction(req.ArticleCtx)
			if err := svc.SessionStore.Create(sessionID, instruction); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to reset session"})
				return
			}

		default:
			// continue mode
			sessionID = req.SessionID
			meta, ok := svc.SessionStore.GetMeta(sessionID)
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"message": "session not found or expired"})
				return
			}
			instruction = meta.Instruction
		}

		// Load history
		history, err := svc.SessionStore.GetMessages(sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to load session history"})
			return
		}

		// Build message list: system + history + user
		var allMessages []adk.Message
		if instruction != "" {
			allMessages = append(allMessages, schema.SystemMessage(instruction))
		}
		allMessages = append(allMessages, history...)
		userMsg := schema.UserMessage(req.UserMessage)
		allMessages = append(allMessages, userMsg)

		// Set SSE headers
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		// Send session_created event for new sessions
		if isNew {
			sseEvent(c, "session_created", ssePayload{SessionID: sessionID})
		}

		// Keep-alive: send heartbeat every 5s to prevent connection timeout
		kaStop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-kaStop:
					return
				case <-ticker.C:
					sseKeepAlive(c)
				}
			}
		}()

		// Run agent
		ctx := c.Request.Context()
		iter := svc.Runner.Run(ctx, allMessages, adk.WithCheckPointID(sessionID))

		var assistantContent strings.Builder

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}

			if event.Err != nil {
				status = "error"
				sseEvent(c, "error", ssePayload{Type: "error", Error: event.Err.Error()})
				break
			}

			// Capture interrupt ID and proposal info, notify frontend
			if event.Action != nil && event.Action.Interrupted != nil {
				for _, ic := range event.Action.Interrupted.InterruptContexts {
					if ic != nil && ic.IsRootCause {
						svc.InterruptIDs.Store(sessionID, ic.ID)
						if proposal, ok := ic.Info.(*tools.ProposalInfo); ok {
							svc.Proposals.Store(sessionID, proposal)
							sseEvent(c, "interrupt", ssePayload{Type: "proposal", Proposal: proposal})
						}
						break
					}
				}
			}

			if event.Output != nil && event.Output.MessageOutput != nil {
				mv := event.Output.MessageOutput
				if mv.IsStreaming && mv.MessageStream != nil {
					stream := mv.MessageStream
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
						if chunk.Role == schema.Assistant && chunk.Content != "" {
							assistantContent.WriteString(chunk.Content)
						}
						p := ssePayload{Type: "stream_chunk", Content: chunk.Content}
						if len(chunk.ToolCalls) > 0 {
							p.ToolCalls = chunk.ToolCalls
						}
						sseEvent(c, "stream_chunk", p)
					}
				} else if mv.Message != nil {
					if mv.Message.Role == schema.Assistant {
						assistantContent.WriteString(mv.Message.Content)
					}
					eventType := "message"
					if mv.Message.Role == schema.Tool {
						eventType = "tool_result"
					}
					p := ssePayload{Type: eventType, Content: mv.Message.Content}
					if len(mv.Message.ToolCalls) > 0 {
						p.ToolCalls = mv.Message.ToolCalls
					}
					sseEvent(c, eventType, p)
				}
			}
		}

		close(kaStop)

		// Persist user + assistant messages
		assistantMsg := schema.AssistantMessage(assistantContent.String(), nil)
		_ = svc.SessionStore.Append(sessionID, userMsg, assistantMsg)

		// Record chat duration
		metrics.AgentChatDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())

		// Send done event
		sseEvent(c, "done", ssePayload{Type: "done"})
	}
}

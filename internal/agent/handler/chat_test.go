package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

func TestBuildInstruction_Nil(t *testing.T) {
	result := buildInstruction(nil)
	if result != "" {
		t.Errorf("expected empty string for nil articleCtx, got %q", result)
	}
}

func TestBuildInstruction_NoContent(t *testing.T) {
	ctx := &articleCtx{}
	result := buildInstruction(ctx)
	if !strings.Contains(result, "coding assistant") {
		t.Errorf("expected base instruction, got %q", result)
	}
}

func TestBuildInstruction_WithArticleContent(t *testing.T) {
	ctx := &articleCtx{
		ArticleID:      "art-1",
		ArticleContent: "This article explains Go interfaces.",
	}
	result := buildInstruction(ctx)
	if !strings.Contains(result, "Article Context") {
		t.Errorf("expected Article Context section, got %q", result)
	}
	if !strings.Contains(result, "Go interfaces") {
		t.Errorf("expected article content in instruction, got %q", result)
	}
}

func TestBuildInstruction_WithCodeBlocks(t *testing.T) {
	ctx := &articleCtx{
		ArticleContent: "Sample article.",
		CodeBlocks: []codeBlock{
			{Language: "go", Code: "fmt.Println(\"hello\")"},
			{Language: "python", Code: "print('hello')"},
		},
	}
	result := buildInstruction(ctx)
	if !strings.Contains(result, "Code Blocks") {
		t.Errorf("expected Code Blocks section, got %q", result)
	}
	if !strings.Contains(result, "Block 1 (go)") {
		t.Errorf("expected Block 1 (go), got %q", result)
	}
	if !strings.Contains(result, "Block 2 (python)") {
		t.Errorf("expected Block 2 (python), got %q", result)
	}
	if !strings.Contains(result, "fmt.Println") {
		t.Errorf("expected go code in instruction, got %q", result)
	}
}

func TestBuildInstruction_ContainsToolInstructions(t *testing.T) {
	ctx := &articleCtx{ArticleContent: "test"}
	result := buildInstruction(ctx)
	if !strings.Contains(result, "Instructions") {
		t.Errorf("expected Instructions section, got %q", result)
	}
}

func newTestGinContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestSSEEvent_StructuredPayload(t *testing.T) {
	c, w := newTestGinContext()

	sseEvent(c, "stream_chunk", ssePayload{
		Type:    "stream_chunk",
		Content: "hello",
	})

	body := w.Body.String()
	if !strings.Contains(body, "event: stream_chunk") {
		t.Errorf("expected event type stream_chunk, got %q", body)
	}

	dataLine := strings.TrimPrefix(strings.Split(body, "\n")[1], "data: ")
	var p ssePayload
	if err := json.Unmarshal([]byte(dataLine), &p); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if p.Type != "stream_chunk" || p.Content != "hello" {
		t.Errorf("unexpected payload: %+v", p)
	}
}

func TestSSEEvent_MessageType(t *testing.T) {
	c, w := newTestGinContext()

	sseEvent(c, "message", ssePayload{
		Type:    "message",
		Content: "full response",
	})

	body := w.Body.String()
	if !strings.Contains(body, "event: message") {
		t.Errorf("expected event type message, got %q", body)
	}
}

func TestSSEEvent_ToolResultType(t *testing.T) {
	c, w := newTestGinContext()

	sseEvent(c, "tool_result", ssePayload{
		Type:    "tool_result",
		Content: "execution output",
	})

	body := w.Body.String()
	if !strings.Contains(body, "event: tool_result") {
		t.Errorf("expected event type tool_result, got %q", body)
	}
}

func TestSSEKeepAlive(t *testing.T) {
	c, w := newTestGinContext()

	sseKeepAlive(c)

	body := w.Body.String()
	if !strings.Contains(body, ": keepalive") {
		t.Errorf("expected keepalive comment, got %q", body)
	}
}

func TestSSEEvent_ErrorType(t *testing.T) {
	c, w := newTestGinContext()

	sseEvent(c, "error", ssePayload{
		Type:  "error",
		Error: "something went wrong",
	})

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected event type error, got %q", body)
	}
	dataLine := strings.TrimPrefix(strings.Split(body, "\n")[1], "data: ")
	var p ssePayload
	json.Unmarshal([]byte(dataLine), &p)
	if p.Error != "something went wrong" {
		t.Errorf("expected error message, got %+v", p)
	}
}

// Verify schema.Message role constants are usable for event type decisions
func TestMessageRoleForEventType(t *testing.T) {
	msg := &schema.Message{Role: schema.Tool, Content: "result"}
	if msg.Role != schema.Tool {
		t.Errorf("expected tool role")
	}
	msg2 := &schema.Message{Role: schema.Assistant, Content: "hi"}
	if msg2.Role != schema.Assistant {
		t.Errorf("expected assistant role")
	}
}

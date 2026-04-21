package feedback_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	ctrl "codeRunner-siwu/internal/interfaces/controller/feedback"
	domainfeedback "codeRunner-siwu/internal/domain/feedback"
)

type mockService struct {
	submitFn func(ctx context.Context, cmd ctrl.SubmitCmd) error
}

func (m *mockService) Submit(ctx context.Context, cmd ctrl.SubmitCmd) error {
	return m.submitFn(ctx, cmd)
}

func newTestRouter(svc ctrl.FeedbackService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/feedback", ctrl.HandleFeedback(svc))
	return r
}

func TestHandleFeedback_Success(t *testing.T) {
	svc := &mockService{submitFn: func(_ context.Context, _ ctrl.SubmitCmd) error { return nil }}
	r := newTestRouter(svc)

	body, _ := json.Marshal(map[string]string{"type": "bug", "content": "这是一条有效的反馈内容"})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "1.2.3.4")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleFeedback_RateLimited(t *testing.T) {
	svc := &mockService{submitFn: func(_ context.Context, _ ctrl.SubmitCmd) error {
		return domainfeedback.ErrRateLimited
	}}
	r := newTestRouter(svc)

	body, _ := json.Marshal(map[string]string{"type": "bug", "content": "这是一条有效的反馈内容"})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestHandleFeedback_InvalidContent(t *testing.T) {
	svc := &mockService{submitFn: func(_ context.Context, _ ctrl.SubmitCmd) error {
		return domainfeedback.ErrInvalidContent
	}}
	r := newTestRouter(svc)

	body, _ := json.Marshal(map[string]string{"type": "bug", "content": "短"})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleFeedback_ServerError(t *testing.T) {
	svc := &mockService{submitFn: func(_ context.Context, _ ctrl.SubmitCmd) error {
		return errors.New("unexpected error")
	}}
	r := newTestRouter(svc)

	body, _ := json.Marshal(map[string]string{"type": "suggestion", "content": "这是一条有效的反馈内容"})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

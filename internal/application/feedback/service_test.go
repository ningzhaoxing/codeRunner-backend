package feedback_test

import (
	"context"
	"errors"
	"testing"

	appfeedback "codeRunner-siwu/internal/application/feedback"
	domainfeedback "codeRunner-siwu/internal/domain/feedback"
)

type mockMailer struct {
	sendFn func(ctx context.Context, subject, body, replyTo string) error
}

func (m *mockMailer) Send(ctx context.Context, subject, body, replyTo string) error {
	return m.sendFn(ctx, subject, body, replyTo)
}

type mockRateLimiter struct {
	allowFn func(ip string) bool
}

func (r *mockRateLimiter) Allow(ip string) bool {
	return r.allowFn(ip)
}

func TestFeedbackService_Submit_Success(t *testing.T) {
	svc := appfeedback.NewService(
		&mockMailer{sendFn: func(ctx context.Context, subject, body, replyTo string) error { return nil }},
		&mockRateLimiter{allowFn: func(ip string) bool { return true }},
		appfeedback.Config{SendTimeout: 0},
	)

	err := svc.Submit(context.Background(), appfeedback.SubmitCmd{
		IP: "1.1.1.1", Type: "bug", Content: "这是一条有效的反馈内容", Contact: "x@y.com",
	})
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestFeedbackService_Submit_RateLimited(t *testing.T) {
	svc := appfeedback.NewService(
		&mockMailer{sendFn: func(_ context.Context, _, _, _ string) error { return nil }},
		&mockRateLimiter{allowFn: func(_ string) bool { return false }},
		appfeedback.Config{},
	)

	err := svc.Submit(context.Background(), appfeedback.SubmitCmd{
		IP: "1.1.1.1", Type: "bug", Content: "这是一条有效的反馈内容",
	})
	if !errors.Is(err, domainfeedback.ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestFeedbackService_Submit_InvalidContent(t *testing.T) {
	svc := appfeedback.NewService(
		&mockMailer{sendFn: func(_ context.Context, _, _, _ string) error { return nil }},
		&mockRateLimiter{allowFn: func(_ string) bool { return true }},
		appfeedback.Config{},
	)

	err := svc.Submit(context.Background(), appfeedback.SubmitCmd{
		IP: "1.1.1.1", Type: "bug", Content: "短",
	})
	if !errors.Is(err, domainfeedback.ErrInvalidContent) {
		t.Errorf("expected ErrInvalidContent, got %v", err)
	}
}

func TestFeedbackService_Submit_MailFail(t *testing.T) {
	svc := appfeedback.NewService(
		&mockMailer{sendFn: func(_ context.Context, _, _, _ string) error { return errors.New("smtp error") }},
		&mockRateLimiter{allowFn: func(_ string) bool { return true }},
		appfeedback.Config{},
	)

	err := svc.Submit(context.Background(), appfeedback.SubmitCmd{
		IP: "1.1.1.1", Type: "suggestion", Content: "这是一条有效的反馈内容",
	})
	if !errors.Is(err, domainfeedback.ErrMailSend) {
		t.Errorf("expected ErrMailSend, got %v", err)
	}
}

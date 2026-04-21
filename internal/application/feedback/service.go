package feedback

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	domainfeedback "codeRunner-siwu/internal/domain/feedback"
	"codeRunner-siwu/internal/infrastructure/mail"
	"go.uber.org/zap"
)

var emailRegexp = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type RateLimiter interface {
	Allow(ip string) bool
}

type Config struct {
	SendTimeout time.Duration
}

type SubmitCmd struct {
	IP      string
	Type    string
	Content string
	Contact string
}

type Service interface {
	Submit(ctx context.Context, cmd SubmitCmd) error
}

type serviceImpl struct {
	mailer mail.Mailer
	rl     RateLimiter
	cfg    Config
	logger *zap.Logger
}

func NewService(mailer mail.Mailer, rl RateLimiter, cfg Config) Service {
	return &serviceImpl{
		mailer: mailer,
		rl:     rl,
		cfg:    cfg,
		logger: zap.L(),
	}
}

func (s *serviceImpl) Submit(ctx context.Context, cmd SubmitCmd) error {
	if !s.rl.Allow(cmd.IP) {
		return domainfeedback.ErrRateLimited
	}

	f := &domainfeedback.Feedback{
		Type:    cmd.Type,
		Content: strings.TrimSpace(cmd.Content),
		Contact: strings.TrimSpace(cmd.Contact),
	}
	if err := f.Validate(); err != nil {
		return err
	}

	timeout := s.cfg.SendTimeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	subject := fmt.Sprintf("[CodeRunner反馈][%s] %s", f.Type, runes(f.Content, 30))
	body := s.buildHTML(cmd.IP, f)
	replyTo := ""
	if emailRegexp.MatchString(f.Contact) {
		replyTo = f.Contact
	}

	if err := s.mailer.Send(sendCtx, subject, body, replyTo); err != nil {
		s.logger.Error("mail send failed", zap.Error(err),
			zap.String("ip", cmd.IP), zap.String("type", cmd.Type),
			zap.Int("content_len", len([]rune(f.Content))))
		return domainfeedback.ErrMailSend
	}
	return nil
}

func (s *serviceImpl) buildHTML(ip string, f *domainfeedback.Feedback) string {
	typeLabel := map[string]string{"bug": "Bug", "suggestion": "建议", "other": "其他"}[f.Type]
	contact := "（未填写）"
	if f.Contact != "" {
		contact = html.EscapeString(f.Contact)
	}
	return fmt.Sprintf(`<pre>
时间：%s
IP：%s
类型：%s
联系方式：%s

---
%s
</pre>`,
		html.EscapeString(time.Now().Format("2006-01-02 15:04:05")),
		html.EscapeString(ip),
		html.EscapeString(typeLabel),
		contact,
		html.EscapeString(f.Content),
	)
}

func runes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

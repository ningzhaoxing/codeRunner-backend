package mail

import (
	"context"
	"fmt"
	"time"

	"gopkg.in/gomail.v2"
)

type SMTPConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	From        string
	To          string
	SendTimeout time.Duration
}

type SMTPMailer struct {
	cfg    SMTPConfig
	dialer *gomail.Dialer
}

func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	d := gomail.NewDialer(cfg.Host, cfg.Port, cfg.Username, cfg.Password)
	return &SMTPMailer{cfg: cfg, dialer: d}
}

func (m *SMTPMailer) Send(ctx context.Context, subject, htmlBody, replyTo string) error {
	msg := gomail.NewMessage()
	msg.SetHeader("From", m.cfg.From)
	msg.SetHeader("To", m.cfg.To)
	msg.SetHeader("Subject", subject)
	if replyTo != "" {
		msg.SetHeader("Reply-To", replyTo)
	}
	msg.SetBody("text/html", htmlBody)

	type result struct{ err error }
	ch := make(chan result, 1)

	go func() {
		ch <- result{err: m.dialer.DialAndSend(msg)}
	}()

	timeout := m.cfg.SendTimeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("mail send cancelled: %w", ctx.Err())
	case <-time.After(timeout):
		return fmt.Errorf("mail send timeout after %s", timeout)
	case r := <-ch:
		return r.err
	}
}

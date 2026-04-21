package mail

import "context"

// Mailer 发送单封 HTML 邮件；replyTo 为空则不设置 Reply-To 头。
type Mailer interface {
	Send(ctx context.Context, subject, htmlBody, replyTo string) error
}

// NoopMailer 是一个不发送任何邮件的空实现，用于开发环境。
type NoopMailer struct{}

func (n *NoopMailer) Send(_ context.Context, _, _, _ string) error {
	return nil
}

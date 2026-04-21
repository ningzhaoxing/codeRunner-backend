package mail

import "context"

// Mailer 发送单封 HTML 邮件；replyTo 为空则不设置 Reply-To 头。
type Mailer interface {
	Send(ctx context.Context, subject, htmlBody, replyTo string) error
}

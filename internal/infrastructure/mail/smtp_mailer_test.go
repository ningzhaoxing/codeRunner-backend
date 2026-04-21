package mail_test

import (
	"context"
	"net"
	"testing"
	"time"

	"codeRunner-siwu/internal/infrastructure/mail"
)

func startMockSMTP(t *testing.T) (host string, port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port, func() { ln.Close() }
}

func TestSMTPMailer_SendTimeout(t *testing.T) {
	host, port, stop := startMockSMTP(t)
	defer stop()

	m := mail.NewSMTPMailer(mail.SMTPConfig{
		Host:        host,
		Port:        port,
		Username:    "test@test.com",
		Password:    "pass",
		From:        "test@test.com",
		To:          "dest@test.com",
		SendTimeout: 200 * time.Millisecond,
	})

	ctx := context.Background()
	err := m.Send(ctx, "subject", "<p>body</p>", "")
	if err == nil {
		t.Error("expected error from unreachable SMTP, got nil")
	}
}

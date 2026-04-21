package initialize

import (
	"context"

	appfeedback "codeRunner-siwu/internal/application/feedback"
	"codeRunner-siwu/internal/application/service/auth"
	"codeRunner-siwu/internal/application/service/client"
	"codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/domain/client/entity"
	domainservice "codeRunner-siwu/internal/domain/server/service"
	"codeRunner-siwu/internal/infrastructure/balanceStrategy/p2cBalance"
	"codeRunner-siwu/internal/infrastructure/common/token"
	"codeRunner-siwu/internal/infrastructure/config"
	docker "codeRunner-siwu/internal/infrastructure/containerBasic"
	"codeRunner-siwu/internal/infrastructure/mail"
	"codeRunner-siwu/internal/infrastructure/ratelimit"
	client2 "codeRunner-siwu/internal/infrastructure/websocket/client"
	"codeRunner-siwu/internal/interfaces/controller"
	ctrlFeedback "codeRunner-siwu/internal/interfaces/controller/feedback"
)

func serverServiceRegister(cfg *config.Config) server.ServerService {
	tokenImpl := token.NewToken()
	tokenSrv := auth.NewService(tokenImpl)
	balanceStrategy := p2cBalance.NewP2CBalancer()
	clientManagerDomain := domainservice.NewClientManagerDomainTmpl(balanceStrategy)
	srv := server.NewServiceImpl(clientManagerDomain)

	feedbackSvc := feedbackServiceRegister(cfg)
	controller.InitSrbInject(srv, tokenSrv, feedbackSvc)
	return srv
}

// feedbackAdapter bridges appfeedback.Service to ctrlFeedback.FeedbackService.
// The two SubmitCmd types are structurally identical but defined in different packages.
type feedbackAdapter struct{ svc appfeedback.Service }

func (a feedbackAdapter) Submit(ctx context.Context, cmd ctrlFeedback.SubmitCmd) error {
	return a.svc.Submit(ctx, appfeedback.SubmitCmd{
		IP:      cmd.IP,
		Type:    cmd.Type,
		Content: cmd.Content,
		Contact: cmd.Contact,
	})
}

func feedbackServiceRegister(cfg *config.Config) ctrlFeedback.FeedbackService {
	rl := ratelimit.NewIPRateLimiter(ratelimit.Config{
		PerMinute: cfg.Feedback.RateLimitPerMin,
		PerDay:    cfg.Feedback.RateLimitPerDay,
	})
	mailer := mail.NewSMTPMailer(mail.SMTPConfig{
		Host:        cfg.Mail.Host,
		Port:        cfg.Mail.Port,
		Username:    cfg.Mail.Username,
		Password:    cfg.Mail.Password,
		From:        cfg.Mail.From,
		To:          cfg.Mail.To,
		SendTimeout: cfg.Mail.SendTimeout,
	})
	svc := appfeedback.NewService(mailer, rl, appfeedback.Config{
		SendTimeout: cfg.Mail.SendTimeout,
	})
	return feedbackAdapter{svc: svc}
}

func clientServiceRegister(poolCfg config.ContainerPoolConfig) (*client.ServiceImpl, *docker.ContainerPool, error) {
	dockerClient := docker.NewDockerClient(poolCfg)
	containerTmpl := docker.NewRunCode(dockerClient)
	websocketClientImpl := client2.NewWebsocketClientImpl()
	InnerServerDomainImpl := entity.NewInnerServerDomainImpl(containerTmpl, websocketClientImpl)
	clientSvr := client.NewServiceImpl(InnerServerDomainImpl)
	return clientSvr, dockerClient.Pool(), nil
}

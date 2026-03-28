package server

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/infrastructure/common/tracing"
	"context"
	"time"

	"go.uber.org/zap"
)

type ServerService interface {
	Execute(ctx context.Context, in *proto.ExecuteRequest) error
	Run(cli WebsocketClient, weight int64) error
}

type ServiceImpl struct {
	ClientManagerDomain
}

func NewServiceImpl(clientManagerDomain ClientManagerDomain) *ServiceImpl {
	return &ServiceImpl{
		ClientManagerDomain: clientManagerDomain,
	}
}

func (w *ServiceImpl) Execute(ctx context.Context, in *proto.ExecuteRequest) error {
	log := tracing.Logger(ctx)

	client, err := w.ClientManagerDomain.GetClientByBalance()
	if err != nil {
		log.Errorw("GetClientByBalance failed", "requestID", in.Id, "err", err)
		return err
	}

	start := time.Now()
	sendErr := client.Send(in)
	w.ClientManagerDomain.Done(client.GetId(), time.Since(start), sendErr)

	if sendErr != nil {
		log.Errorw("Send failed", "requestID", in.Id, "clientID", client.GetId(), "err", sendErr)
		return sendErr
	}

	log.Infow("request dispatched", "requestID", in.Id, "clientID", client.GetId())
	// Send 成功后记录为在途请求，等待 Client 的 ACK
	w.ClientManagerDomain.TrackRequest(client.GetId(), in.Id, in)
	return nil
}

func (w *ServiceImpl) Run(cli WebsocketClient, weight int64) error {
	client := entity.NewClient(cli)

	// 注册 ACK 回调：收到 ACK 时从 pendingReqs 移除
	cli.SetAckHandler(func(requestID string) {
		w.ClientManagerDomain.AcknowledgeRequest(client.GetId(), requestID)
	})

	w.ClientManagerDomain.AddClient(client, weight)

	if err := client.HeartBeat(); err != nil {
		zap.S().Errorw("HeartBeat failed", "clientID", client.GetId(), "err", err)
		return err
	}

	for {
		if _, err := client.Read(); err != nil {
			zap.S().Errorw("Read failed", "clientID", client.GetId(), "err", err)

			// 断线：取出所有未 ACK 的请求并重新分发
			pending := w.ClientManagerDomain.DrainPending(client.GetId())
			if len(pending) > 0 {
				zap.S().Warnf("application.server.Run() client %s disconnected with %d pending requests, redispatching", client.GetId(), len(pending))
				for _, req := range pending {
					// 重发时生成新的 TraceID，与原链路区分
					redispatchCtx := tracing.WithTraceID(context.Background(), tracing.NewTraceID())
					if redispatchErr := w.Execute(redispatchCtx, req); redispatchErr != nil {
						zap.S().Errorf("application.server.Run() redispatch failed for request %s: %v", req.Id, redispatchErr)
					}
				}
			}

			return err
		}
	}
}

type ClientManagerDomain interface {
	AddClient(*entity.Client, int64)
	RemoveClient(string) error
	GetClientByBalance() (*entity.Client, error)
	GetClientById(id string) (*entity.Client, error)
	Done(id string, duration time.Duration, err error)
	TrackRequest(clientID, requestID string, req *proto.ExecuteRequest)
	AcknowledgeRequest(clientID, requestID string)
	DrainPending(clientID string) []*proto.ExecuteRequest
}

type WebsocketClient interface {
	Send(requestID string, payload []byte) error
	SetAckHandler(fn func(requestID string))
	Close() error
	HeartBeat() error
	Read() ([]byte, error)
	IsClosed() bool
}

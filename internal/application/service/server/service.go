package server

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity"
	"fmt"
	"time"

	"go.uber.org/zap"
)

type ServerService interface {
	Execute(in *proto.ExecuteRequest) error
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

func (w *ServiceImpl) Execute(in *proto.ExecuteRequest) error {
	client, err := w.ClientManagerDomain.GetClientByBalance()
	if err != nil {
		zap.S().Error(fmt.Sprintln("application.server.Execute() GetClientByBalance err=\n", err))
		return err
	}

	start := time.Now()
	sendErr := client.Send(in)
	w.ClientManagerDomain.Done(client.GetId(), time.Since(start), sendErr)

	if sendErr != nil {
		zap.S().Error(fmt.Sprintln("application.server.Execute() Send err=\n", sendErr))
		return sendErr
	}

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
		zap.S().Error(fmt.Sprintln("application.server.Run() HeartBeat err=\n", err))
		return err
	}

	for {
		if _, err := client.Read(); err != nil {
			zap.S().Error(fmt.Sprintln("application.server.Run() Read() err=\n", err))

			// 断线：取出所有未 ACK 的请求并重新分发
			pending := w.ClientManagerDomain.DrainPending(client.GetId())
			if len(pending) > 0 {
				zap.S().Warnf("application.server.Run() client %s disconnected with %d pending requests, redispatching", client.GetId(), len(pending))
				for _, req := range pending {
					if redispatchErr := w.Execute(req); redispatchErr != nil {
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

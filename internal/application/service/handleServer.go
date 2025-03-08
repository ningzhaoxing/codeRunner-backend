package service

import (
	"codeRunner-siwu/internal/domain/loadBalance/entity"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy"
	"codeRunner-siwu/internal/infrastructure/common/utils"
	"github.com/gorilla/websocket"
)

type HandleServer struct {
	entity.LoadBalanceDomain
}

func NewHandleServer() *HandleServer {
	return &HandleServer{
		LoadBalanceDomain: entity.NewWeightedRR(),
	}
}

// RegisterServerLoadBalance 将服务器注册到负载均衡
func (h *HandleServer) RegisterServerLoadBalance(conn *websocket.Conn) error {
	serverId, err := utils.GetUuid()
	if err != nil {
		return err
	}
	h.AddServer(bananceStrategy.NewWeightNode(serverId, conn, 1))

	return nil
}

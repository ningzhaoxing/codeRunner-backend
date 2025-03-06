package initialize

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/serviceRegistry"
	"context"
	"fmt"
	"time"
)

func InitEtcd(ctx context.Context, c *config.Config) (*serviceRegistry.EtcdRegistry, error) {
	registerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// 注册grpc
	etcdClient, err := serviceRegistry.NewEtcdRegistry([]string{c.Etcd.Endpoints}, c.Etcd.Key, fmt.Sprintf("%s:%s", c.Grpc.Host, c.Grpc.Port))
	if err != nil {
		return nil, err
	}

	// 注册
	if err := etcdClient.Register(registerCtx, 3); err != nil {
		return nil, err
	}

	return etcdClient, nil
}

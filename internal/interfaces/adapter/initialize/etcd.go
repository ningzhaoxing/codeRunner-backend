package initialize

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/serviceRegistry"
	"context"
	"fmt"
	"log"
	"time"
)

func EtcdRegister(ctx context.Context, c *config.Config) (*serviceRegistry.EtcdRegistry, error) {
	registerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 注册grpc
	endPoints := []string{fmt.Sprintf("http://%s", c.Etcd.Endpoints)}

	etcdClient, err := serviceRegistry.NewEtcdRegistry(endPoints, c.Etcd.Key, fmt.Sprintf("%s:%s", c.Grpc.Host, c.Grpc.Port))
	if err != nil {
		log.Println("interfaces-adapter-initialize-etcd EtcdRegister的serviceRegistry.NewEtcdRegistry err=", err)
		return nil, err
	}

	if err := etcdClient.Register(registerCtx, 3); err != nil {
		log.Println("interfaces-adapter-initialize-etcd EtcdRegister的etcdClient.Register err=", err)
		return nil, err
	}

	return etcdClient, nil
}

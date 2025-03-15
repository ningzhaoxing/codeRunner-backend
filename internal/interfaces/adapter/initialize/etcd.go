package initialize

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/serviceRegistry"
	"context"
	"fmt"
	"log"
)

func EtcdRegister(ctx context.Context, c *config.Config) (*serviceRegistry.EtcdRegistry, error) {
	// 注册grpc

	// 鏈接etcd
	endPoints := []string{fmt.Sprintf("http://%s", c.Etcd.Endpoints)}
	etcdClient, err := serviceRegistry.NewEtcdRegistry(endPoints, c.Etcd.Key, fmt.Sprintf("%s:%s", "8.154.36.180", c.Grpc.Port))
	if err != nil {
		log.Println("interfaces-adapter-initialize-etcd EtcdRegister的serviceRegistry.NewEtcdRegistry err=", err)
		return nil, err
	}

	fmt.Println("etcd鏈接成功!")

	if err := etcdClient.Register(ctx, 10); err != nil {
		log.Println("interfaces-adapter-initialize-etcd EtcdRegister的etcdClient.Register err=", err)
		return nil, err
	}

	return etcdClient, nil
}

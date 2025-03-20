package initialize

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/etcd"
	"context"
	"fmt"
	"log"
)

func EtcdRegister(ctx context.Context, c *config.Config) (*etcd.EtcdRegistry, error) {
	// 鏈接etcd
	endPoints := []string{fmt.Sprintf("http://%s", c.Server.Etcd.Endpoints)}
	etcdClient, err := etcd.NewEtcdRegistry(endPoints, c.Server.Etcd.Key, fmt.Sprintf("%s:%s", "0.0.0.0", c.Server.Grpc.Port))
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

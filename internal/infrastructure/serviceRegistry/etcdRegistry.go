package serviceRegistry

import (
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log"
	"time"
)

type EtcdRegistry struct {
	Client  *clientv3.Client
	leaseID clientv3.LeaseID
	key     string
	value   string
}

func NewEtcdRegistry(endpoints []string, key, value string) (*EtcdRegistry, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		log.Println("infrastructure-serviceRegistry NewEtcdRegistry的clientv3.New err=", err)
		return nil, err
	}
	return &EtcdRegistry{
		Client: cli,
		key:    key,
		value:  value,
	}, nil
}

// Register 注册服务
func (r *EtcdRegistry) Register(ctx context.Context, ttl int64) error {
	// 1. 创建租约
	lease := clientv3.NewLease(r.Client)

	grantResp, err := lease.Grant(ctx, ttl)
	if err != nil {
		log.Println("infrastructure-serviceRegistry Register的lease.Grant err=", err)
		return err
	}
	r.leaseID = grantResp.ID

	// 2. 绑定租约并写入 key-value
	_, err = r.Client.Put(ctx, r.key, r.value, clientv3.WithLease(r.leaseID))
	if err != nil {
		log.Println("infrastructure-serviceRegistry Register的r.Client.Put err=", err)
		return err
	}

	fmt.Println("etcd服務注冊成功！")

	// 3. 自动续约
	keepAliveCh, err := lease.KeepAlive(ctx, r.leaseID)
	if err != nil {
		log.Println("infrastructure-serviceRegistry Register的lease.KeepAlive err=", err)
		return err
	}

	// 4. 监听续约响应
	go func() {
		for range keepAliveCh {
		}
	}()
	return nil
}

// Unregister 注销服务
func (r *EtcdRegistry) Unregister(ctx context.Context) error {
	defer func(Client *clientv3.Client) {
		err := Client.Close()
		if err != nil {
			log.Println("infrastructure-serviceRegistry Unregister的Client.Close() err=", err)
			return
		}
	}(r.Client)
	_, err := r.Client.Revoke(ctx, r.leaseID)
	if err != nil {
		log.Println("infrastructure-serviceRegistry Unregister的r.Client.Revoke err=", err)
		return err
	}
	return nil
}

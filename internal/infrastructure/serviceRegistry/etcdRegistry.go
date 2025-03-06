package serviceRegistry

import (
	"context"
	clientv3 "go.etcd.io/etcd/client/v3"
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
		return err
	}
	r.leaseID = grantResp.ID

	// 2. 绑定租约并写入 key-value
	_, err = r.Client.Put(ctx, r.key, r.value, clientv3.WithLease(r.leaseID))
	if err != nil {
		return err
	}

	// 3. 自动续约
	keepAliveCh, err := lease.KeepAlive(ctx, r.leaseID)
	if err != nil {
		return err
	}

	// 4. 监听续约响应
	go func() {
		for range keepAliveCh {
			// 续约成功，若通道关闭则需重新注册
		}
	}()
	return nil
}

// Unregister 注销服务
func (r *EtcdRegistry) Unregister(ctx context.Context) error {
	defer func(Client *clientv3.Client) {
		err := Client.Close()
		if err != nil {

		}
	}(r.Client)
	_, err := r.Client.Revoke(ctx, r.leaseID)
	return err
}

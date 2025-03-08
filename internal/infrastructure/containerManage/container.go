package containerManage

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type dockerContainerClient struct {
	ctx context.Context
	cli *client.Client
}

// NewDockerClient 新构造函数：通过完整host地址连接
func NewDockerClient(ctx context.Context, ip, port string) (*dockerContainerClient, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(fmt.Sprintf("tcp://%s:%s", ip, port)),
		client.WithAPIVersionNegotiation(), // 自动协商API版本
	)
	if err != nil {
		return nil, fmt.Errorf("创建Docker客户端失败: %v", err)
	}
	return &dockerContainerClient{ctx: ctx, cli: cli}, nil
}

// CreateContainer 创建指定容器
func (client *dockerContainerClient) createContainer(image string) (createContain container.CreateResponse, err error) {
	config := &container.Config{Image: image}
	createContain, err = client.cli.ContainerCreate(client.ctx, config, nil, nil, nil, "")
	if err != nil {
		return container.CreateResponse{}, fmt.Errorf("创建容器失败:%v", err)
	}
	return
}

// RmContainer 删除指定id容器
func (client *dockerContainerClient) rmContainer(id string) error {
	// 设置删除选项
	option := container.RemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   true,
		Force:         true,
	}
	err := client.cli.ContainerRemove(client.ctx, id, option)
	if err != nil {
		return fmt.Errorf("删除容器失败:%v", err)
	}
	return nil
}

// StopContainer 停止指定id容器
func (client *dockerContainerClient) stopContainer(id string) error {
	err := client.cli.ContainerStop(client.ctx, id, container.StopOptions{})
	if err != nil {
		return fmt.Errorf("停止容器失败:%v", err)
	}
	return nil
}

func (client *dockerContainerClient) RunCode() {

}

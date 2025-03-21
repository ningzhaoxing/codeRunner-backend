package containerBasic

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"log"
	"os"
	"strings"
	"time"
)

type ContainerSrv interface {
	GetContains(language string) (container string)
	EnsureContainerExists(language string) error
}

type containerSrvImpl struct {
	err error
	//记录支持的语言类型
	language []string
	//支持的镜像
	images map[string]string
	cli    *client.Client
	ctx    context.Context
}

func (c *containerSrvImpl) GetContains(language string) (container string) {
	return c.images[language]
}

func (c *containerSrvImpl) EnsureContainerExists(language string) error {
	containerName := c.images[language]

	// 检查容器是否存在
	args := filters.NewArgs()
	args.Add("name", containerName)

	containers, err := c.cli.ContainerList(c.ctx, types.ContainerListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		return fmt.Errorf("检查容器失败: %v", err)
	}

	if len(containers) == 0 {
		log.Printf("容器 %s 不存在，正在创建...", containerName)
		return c.createContainer(language, c.images[language], containerName).err
	}

	// 检查容器是否运行中
	if containers[0].State != "running" {
		log.Printf("容器 %s 未运行，正在启动...", containerName)
		if err := c.cli.ContainerStart(c.ctx, containers[0].ID, types.ContainerStartOptions{}); err != nil {
			return fmt.Errorf("启动容器失败: %v", err)
		}
		log.Printf("容器 %s 已启动", containerName)
	}

	return nil
}

func NewContainerSrvImpl() *containerSrvImpl {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix:///var/run/docker.sock"),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		log.Fatalf("创建Docker客户端失败: %v", err)
		return nil
	}

	language := []string{"go", "c", "java", "python", "js"}
	images := map[string]string{
		"go":     "code-runner-go",
		"java":   "code-runner-java",
		"c":      "code-runner-cpp",
		"python": "code-runner-python",
		"js":     "code-runner-js",
	}

	containerSrvImpl := containerSrvImpl{
		err:      nil,
		language: language,
		images:   images,
		cli:      cli,
		ctx:      context.Background(),
	}

	// 创建目录
	containerSrvImpl.createContent()

	// 确保每个语言的容器都存在
	for _, lang := range language {
		err := containerSrvImpl.EnsureContainerExists(lang)
		if err != nil {
			log.Printf("确保 %s 容器存在时出错: %v", lang, err)
			containerSrvImpl.err = err
		}
	}

	return &containerSrvImpl
}

// 创建单个容器
func (c *containerSrvImpl) createContainer(language, image, name string) *containerSrvImpl {
	if c.err != nil {
		return c
	}

	// 检查镜像是否存在
	_, _, err := c.cli.ImageInspectWithRaw(c.ctx, image)
	if err != nil {
		log.Printf("镜像 %s 不存在，请先构建镜像", image)
		c.err = fmt.Errorf("镜像 %s 不存在: %v", image, err)
		return c
	}

	// 创建目录
	tmpDir := fmt.Sprintf("/app/tmp/%s", language)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		log.Printf("创建目录 %s 失败: %v", tmpDir, err)
		c.err = err
		return c
	}

	// 准备挂载卷
	mounts := []string{fmt.Sprintf("%s:/app/tmp/%s", tmpDir, language)}

	// 创建容器配置
	config := &container.Config{
		Image: image,
		Cmd:   []string{"sleep", "infinity"},
	}

	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode("none"),
		Binds:       mounts,
	}

	// 创建容器
	resp, err := c.cli.ContainerCreate(c.ctx, config, hostConfig, nil, nil, name)
	if err != nil {
		// 检查是否因为容器已存在而失败
		if strings.Contains(err.Error(), "already in use") {
			log.Printf("容器 %s 已存在，尝试重启", name)

			// 查找容器ID
			args := filters.NewArgs()
			args.Add("name", name)
			containers, listErr := c.cli.ContainerList(c.ctx, types.ContainerListOptions{
				All:     true,
				Filters: args,
			})
			if listErr != nil {
				log.Printf("查找容器失败: %v", listErr)
				c.err = listErr
				return c
			}

			if len(containers) > 0 {
				// 重启容器
				if restartErr := c.cli.ContainerRestart(c.ctx, containers[0].ID, container.StopOptions{}); restartErr != nil {
					log.Printf("重启容器失败: %v", restartErr)
					c.err = restartErr
				}
			}
			return c
		}

		log.Printf("创建容器失败: %v", err)
		c.err = err
		return c
	}

	// 启动容器
	if err := c.cli.ContainerStart(c.ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Printf("启动容器失败: %v", err)
		c.err = err
		return c
	}

	log.Printf("容器 %s 创建并启动成功: %s", name, resp.ID)
	return c
}

// 创建对应的文件夹
func (c *containerSrvImpl) createContent() *containerSrvImpl {
	// 在/app/tmp下创建文件夹
	for i := 0; i < len(c.language); i++ {
		tempDir := fmt.Sprintf("/app/tmp/%s", c.language[i])
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			log.Printf("创建目录 %s 失败: %v", tempDir, err)
			c.err = err
			return c
		}
		log.Printf("创建目录成功: %s", tempDir)
	}
	return c
}

// CreateAllContainers 创建多个容器,容器名字跟构建的镜像名字一样
func (c *containerSrvImpl) createAllContainers() error {
	//构建镜像
	for i := 0; i < len(c.language); i++ {
		lau := c.language[i]
		c.createContainer(lau, c.images[lau], c.images[lau])
		if c.err != nil {
			log.Printf("创建 %s 容器失败: %v", lau, c.err)
			return c.err
		}
		// 等待一秒确保容器启动
		time.Sleep(1 * time.Second)
	}
	return c.err
}

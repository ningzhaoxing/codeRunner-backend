package containerBasic

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

type ContainerSrv interface {
	GetContains(language string) (container string)
}

type containerSrvImpl struct {
	err error
	//记录支持的语言类型
	language []string
	//支持的镜像
	images map[string]string
}

func (c *containerSrvImpl) GetContains(language string) (container string) {
	return c.images[language]
}

func NewContainerSrvImpl() *containerSrvImpl {
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
	}
	containerSrvImpl.err = containerSrvImpl.createAllContainers()
	return &containerSrvImpl
}

// 创建单个容器
func (c *containerSrvImpl) createContainer(language, image, name string) *containerSrvImpl {
	if c.err != nil {
		return c
	}
	volume := fmt.Sprintf("/tmp/%s:/app", language)
	//创建常驻容器
	cmd := exec.Command("docker", "run", "-d", "--name", name, "--network", "none", "-v", volume, image, "sleep", "infinity")
	var out bytes.Buffer
	cmd.Stdout = &out
	c.err = cmd.Run()
	return c
}

// 创建对应的文件夹
func (c *containerSrvImpl) createContent() *containerSrvImpl {
	// 在/app/tmp下创建六个文件夹
	for i := 0; i < len(c.language); i++ {
		tempDir := fmt.Sprintf("/app/tmp/%s", c.language[i])
		c.err = os.MkdirAll(tempDir, 0755)
	}
	return c
}

// CreateAllContainers 创建多个容器,容器名字跟构建的镜像名字一样
func (c *containerSrvImpl) createAllContainers() error {

	//创建目录
	c.createContent()
	//构建镜像
	for i := 0; i < len(c.language); i++ {
		lau := c.language[i]
		c.createContainer(lau, c.images[lau], c.images[lau])
	}
	return c.err
}

package errors

import "errors"

var (
	ErrContainerExecTimeout   = errors.New("容器执行超时")
	ErrContainerPoolExhausted = errors.New("容器池资源耗尽")
	ErrContainerPoolClosed    = errors.New("容器池已关闭")
	ErrUnsupportedLanguage    = errors.New("不支持的语言")
)

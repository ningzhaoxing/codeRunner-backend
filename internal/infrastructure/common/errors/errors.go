package errors

import errors2 "errors"

var (
	UserClientNotExist      = errors2.New("该用户客户端不存在")
	UserClientHasExist      = errors2.New("该用户客户端已存在")
	NotFoundEffectiveServer = errors2.New("未找到可用的服务器")
	ResultSendFail          = errors2.New("运行结果发送失败")
	MaxRetryAttemptsReached = errors2.New("已超过最大重连次数")
)

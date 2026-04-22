package errors

import errors2 "errors"

var (
	NotFoundEffectiveServer = errors2.New("未找到可用的服务器")
	ResultSendFail          = errors2.New("运行结果发送失败")
	MaxRetryAttemptsReached = errors2.New("已超过最大重连次数")
	ClientClosed            = errors2.New("websocket client closed")
)

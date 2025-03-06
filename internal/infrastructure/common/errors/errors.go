package errors

import errors2 "errors"

var (
	UserClientNotExist = errors2.New("该用户客户端不存在")
	UserClientHasExist = errors2.New("该用户客户端已存在")
)

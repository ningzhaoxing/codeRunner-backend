package feedback

import "errors"

var (
	ErrInvalidType    = errors.New("反馈类型无效")
	ErrInvalidContent = errors.New("内容长度需在 10-2000 字符之间")
	ErrInvalidContact = errors.New("联系方式长度不能超过 100 字符")
	ErrRateLimited    = errors.New("提交过于频繁，请稍后再试")
	ErrMailSend       = errors.New("发送失败，请稍后重试")
)

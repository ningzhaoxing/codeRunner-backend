package tracing

import (
	"context"
	"crypto/rand"
	"fmt"

	"go.uber.org/zap"
)

type contextKey string

const traceIDKey contextKey = "traceID"

// NewTraceID 生成一个 16 字节十六进制的随机 TraceID
func NewTraceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// WithTraceID 将 traceID 注入 context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// FromContext 从 context 中取出 traceID，不存在则返回空字符串
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey).(string); ok {
		return id
	}
	return ""
}

// Logger 返回携带 traceID 字段的 SugaredLogger；没有 traceID 时等同于 zap.S()
func Logger(ctx context.Context) *zap.SugaredLogger {
	traceID := FromContext(ctx)
	if traceID == "" {
		return zap.S()
	}
	return zap.S().With("traceID", traceID)
}

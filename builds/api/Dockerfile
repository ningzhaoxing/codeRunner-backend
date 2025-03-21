FROM golang:1.23-alpine AS builder

WORKDIR /app

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    TZ=Asia/Shanghai

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN go build -o code-runner ./cmd/api

# 使用轻量级的 alpine 作为最终镜像
FROM alpine:latest

WORKDIR /app
COPY --from=builder --chown=runner /app/code-runner .
COPY --chown=runner configs/config.yaml ./configs/

# 设置时区
RUN apk --no-cache add tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone && \
    apk del tzdata

ENTRYPOINT ["/app/code-runner"]
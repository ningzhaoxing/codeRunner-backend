FROM golang:1.21-alpine


# 设置工作目录为 /app（Docker 会自动创建）
WORKDIR /app

# 将 /app 的所有权交给 root 用户
RUN chown -R root:root /app


# 设置环境变量
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    TZ=Asia/Shanghai
FROM golang:1.21-alpine

# 创建用户 runner，并设置 UID 为 10000
RUN adduser -D -u 10000 runner

# 设置工作目录为 /app（Docker 会自动创建）
WORKDIR /app

# 将 /app 的所有权交给 runner 用户
RUN chown -R runner:runner /app

# 切换到 runner 用户
USER runner

# 设置环境变量
ENV GO111MODULE=on \
    CGO_ENABLED=0

# 容器启动时运行命令
CMD ["go", "run", "main.go"]
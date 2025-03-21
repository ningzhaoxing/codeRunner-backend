FROM golang:1.21-alpine

# 设置工作目录
WORKDIR /app

# 创建tmp目录
RUN mkdir -p /app/tmp/go

# 不使用缓存
ENV GOPROXY=direct
ENV GO111MODULE=auto

# 安装必要的工具
RUN apk add --no-cache tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 容器启动命令
CMD ["sleep", "infinity"] 
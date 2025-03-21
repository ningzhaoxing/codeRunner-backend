FROM openjdk:17-alpine

# 设置工作目录
WORKDIR /app

# 创建tmp目录
RUN mkdir -p /app/tmp/java

# 安装必要的工具
RUN apk add --no-cache tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 容器启动命令
CMD ["sleep", "infinity"] 
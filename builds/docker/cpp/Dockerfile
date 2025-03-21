FROM gcc:latest

# 设置工作目录
WORKDIR /app

# 创建tmp目录
RUN mkdir -p /app/tmp/c

# 安装必要的工具
RUN apt-get update && apt-get install -y tzdata && \
    ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone && \
    apt-get clean

# 容器启动命令
CMD ["sleep", "infinity"] 
FROM node:20-alpine

# 创建一个用户和目录
RUN useradd -m -u 10000 runner && \
    mkdir -p /app && \
    chown -R runner:runner /app

# 设置工作目录
WORKDIR /app

# 切换到普通用户
USER runner

# 设置默认命令，编译并运行 JavaScript 文件
CMD ["sh", "-c", "node main.js"]
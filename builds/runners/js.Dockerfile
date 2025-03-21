# 使用Node.js官方镜像
FROM node:20-slim

# 创建一个名为runner的用户，用于运行代码
RUN adduser --disabled-password --gecos "" --uid 10000 --home /home/runner runner

# 设置工作目录为/app
WORKDIR /app

# 将/app目录的所有权交给runner用户
RUN chown -R runner:runner /app

# 切换到runner用户
USER runner
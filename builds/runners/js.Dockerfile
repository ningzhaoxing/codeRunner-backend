FROM node:20-alpine

# 创建用户 runner，并设置 UID 为 10000
RUN useradd -m -u 10000 runner

# 设置工作目录为 /app（Docker 会自动创建）
WORKDIR /app

# 将 /app 的所有权交给 runner 用户
RUN chown -R runner:runner /app

# 切换到普通用户
USER runner

# 设置默认命令，运行 JavaScript 文件
CMD ["sh", "-c", "node main.js"]
FROM openjdk:21-jdk-slim

# 使用 adduser 创建用户 runner
RUN adduser --disabled-password --gecos "" --uid 10000 --home /home/runner runner

# 设置工作目录为 /app（Docker 会自动创建）
WORKDIR /app

# 将 /app 的所有权交给 runner 用户
RUN chown -R runner:runner /app

# 切换到 runner 用户
USER runner

# 容器启动时运行命令
CMD ["sh", "-c", "javac Main.java && java Main"]
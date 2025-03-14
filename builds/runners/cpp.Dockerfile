FROM gcc:12.2.0

# 创建用户 runner，并设置 UID 为 10000
RUN useradd -m -u 10000 runner

# 设置工作目录为 /app（Docker 会自动创建）
WORKDIR /app

# 将 /app 的所有权交给 runner 用户
RUN chown -R runner:runner /app

# 切换到 runner 用户
USER runner

# 容器启动时运行命令
CMD ["sh", "-c", "g++ -Wall -Wextra -Werror -O2 -std=c++20 -o main main.cpp && ./main"]
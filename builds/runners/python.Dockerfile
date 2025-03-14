FROM python:3.11-slim

# 创建用户 runner，并清理 apt 缓存
RUN useradd -m -u 10000 runner && \
    rm -rf /var/lib/apt/lists/*

# 设置工作目录为 /app（Docker 会自动创建）
WORKDIR /app

# 将 /app 的所有权交给 runner 用户
RUN chown -R runner:runner /app

# 切换到普通用户
USER runner

# 设置环境变量
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

# 设置默认命令，运行 Python 文件
CMD ["python3", "main.py"]
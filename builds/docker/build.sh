#!/bin/bash

# 构建Go镜像
echo "构建Go镜像..."
docker build -t code-runner-go -f builds/docker/go/Dockerfile .

# 构建Java镜像
echo "构建Java镜像..."
docker build -t code-runner-java -f builds/docker/java/Dockerfile .

# 构建C++镜像
echo "构建C++镜像..."
docker build -t code-runner-cpp -f builds/docker/cpp/Dockerfile .

# 构建Python镜像
echo "构建Python镜像..."
docker build -t code-runner-python -f builds/docker/python/Dockerfile .

# 构建Node.js镜像
echo "构建Node.js镜像..."
docker build -t code-runner-js -f builds/docker/js/Dockerfile .

echo "所有镜像构建完成！" 
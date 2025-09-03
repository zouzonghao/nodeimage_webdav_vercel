#!/bin/bash

# 设置 -e 选项，如果命令返回非零退出状态，则立即退出
set -e

# Docker Hub 用户名和镜像名称
DOCKER_USERNAME="sanqi37"
IMAGE_NAME="nodeimage_webdav_webui"

# 获取版本号
# 优先从第一个参数获取，否则提示用户输入
if [ -n "$1" ]; then
  VERSION=$1
else
  read -p "请输入版本号 (例如: 1.0.0): " VERSION
fi

if [ -z "$VERSION" ]; then
  echo "错误：版本号不能为空。"
  exit 1
fi

# 完整的镜像标签
IMAGE_TAG_VERSIONED="$DOCKER_USERNAME/$IMAGE_NAME:$VERSION"
IMAGE_TAG_LATEST="$DOCKER_USERNAME/$IMAGE_NAME:latest"

# 步骤 1: 在本地构建 Go 应用程序 (为 Linux 环境)
echo "正在为 Linux 环境构建 Go 应用程序..."
CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s" -o main .
echo "构建完成。"

# 步骤 2: 构建 Docker 镜像
echo "正在构建 Docker 镜像: $IMAGE_TAG_VERSIONED"
docker build -t "$IMAGE_TAG_VERSIONED" .

# 步骤 3: 给镜像打上 latest 标签
echo "正在标记镜像为: $IMAGE_TAG_LATEST"
docker tag "$IMAGE_TAG_VERSIONED" "$IMAGE_TAG_LATEST"

# 步骤 4: 清理本地构建的二进制文件
rm main

# 提示用户登录 Docker Hub
echo ""
echo "请确保您已登录到 Docker Hub。"
echo "您可以使用 'docker login' 命令进行登录。"
# docker login # 或者取消注释此行以强制登录

# 推送带版本号的镜像
echo "正在推送镜像: $IMAGE_TAG_VERSIONED"
docker push "$IMAGE_TAG_VERSIONED"

# 推送 latest 镜像
echo "正在推送镜像: $IMAGE_TAG_LATEST"
docker push "$IMAGE_TAG_LATEST"

echo ""
echo "🎉 镜像发布成功!"
echo "版本: $VERSION"
echo "镜像: $IMAGE_TAG_VERSIONED"
echo "       $IMAGE_TAG_LATEST"
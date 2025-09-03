# 使用一个轻量级的镜像作为最终的运行环境
FROM alpine:latest

# 安装时区数据和 CA 证书
RUN apk add --no-cache tzdata ca-certificates
ENV TZ=Asia/Shanghai

# 设置工作目录
WORKDIR /app

# 复制预先构建好的二进制文件和 public 目录
# 注意：这些文件需要在运行 'docker build' 之前在本地准备好
COPY main ./
COPY public ./public

# 暴露端口
EXPOSE 37372

# 设置容器启动时执行的命令
CMD ["./main"]
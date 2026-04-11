# 使用官方Go镜像作为构建环境
FROM golang:1.21-alpine AS builder

# 与本地/Makefile 一致，避免部分网络环境下 go mod 失败
ENV GOPROXY=https://goproxy.io,https://proxy.golang.org,direct

# 设置工作目录
WORKDIR /app

# 复制go.mod和go.sum文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -o circle-go ./cmd/api

# 使用轻量级的Alpine镜像作为运行环境
FROM alpine:latest

# 安装必要的包
RUN apk --no-cache add ca-certificates

# 设置工作目录
WORKDIR /app

# 从构建阶段复制编译好的应用
COPY --from=builder /app/circle-go .

# 复制配置文件和前端文件
COPY config/config.yaml ./config/
COPY frontend/ ./frontend/

# 暴露端口
EXPOSE 8080

# 设置环境变量
ENV CONFIG_PATH=/app/config/config.yaml

# 运行应用
CMD ["./circle-go", "--config", "/app/config/config.yaml"]

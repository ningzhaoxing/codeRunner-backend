FROM golang:1.23-alpine AS builder
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    TZ=Asia/Shanghai

WORKDIR /app
COPY . .

RUN go mod download

RUN go build -o code-runner ./cmd/api

# 运行阶段增加证书
FROM alpine:3.17

WORKDIR /app
COPY --from=builder --chown=runner /app/code-runner .
COPY --chown=runner configs/configOS.yaml ./configs/

EXPOSE 8080
ENTRYPOINT ["/app/code-runner"]
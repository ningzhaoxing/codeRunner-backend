FROM golang:1.23-alpine AS builder

WORKDIR /app

ENV GO111MODULE=on \
    GOPROXY=https://mirrors.aliyun.com/goproxy/,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    TZ=Asia/Shanghai

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o code-runner ./cmd/api/main.go

FROM alpine:latest

RUN adduser -D -u 10000 runner

WORKDIR /app
COPY --from=builder --chown=runner /app/code-runner .
COPY --chown=runner configs/dev.yaml ./configs/
COPY --chown=runner configs/product.yaml ./configs/

USER runner

CMD ["/app/code-runner"]

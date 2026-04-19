FROM golang:1.25-alpine AS builder

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
COPY --from=builder /app/code-runner .
COPY configs/dev.yaml ./configs/
COPY configs/product.yaml ./configs/

RUN chown -R runner:runner /app

USER runner

CMD ["/app/code-runner"]

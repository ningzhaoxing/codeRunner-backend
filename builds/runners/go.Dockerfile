FROM golang:1.21-alpine

RUN adduser -D -u 10000 runner && \
    mkdir -p /app && \
    chown -R runner:runner /app

WORKDIR /app
USER runner

ENV GO111MODULE=on \
    CGO_ENABLED=0

CMD ["go", "run", "main.go"]
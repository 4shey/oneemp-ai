FROM golang:1.26.3-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/setup ./cmd/setup

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/bot ./bot
COPY --from=builder /out/setup ./setup
COPY prompts ./prompts

USER app

EXPOSE 8080

ENTRYPOINT ["./bot"]

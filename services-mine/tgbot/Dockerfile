# ---------- build stage ----------
# FROM golang:1.22-alpine AS builder
FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git

# зависимости — отдельно для кеша
COPY go.mod go.sum ./
RUN go mod download

# исходники
COPY . .

# собираем пакет ./bot (где main.go)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/tgbot ./cmd/bot


# ---------- runtime stage ----------
FROM alpine:3.20

# Часто нужно:
# - ca-certificates: https запросы
# - bash: если скрипты используют bash (или shebang #!/bin/bash)
RUN apk add --no-cache ca-certificates bash && update-ca-certificates

WORKDIR /app

# бинарник
COPY --from=builder /out/tgbot ./tgbot

# целиком папку scripts
# COPY --from=builder /src/scripts ./scripts

# на всякий случай сделаем .sh исполняемыми
# RUN chmod -R +x ./scripts

# если твой бот слушает порт — можно EXPOSE, но необязательно
# EXPOSE 8080

ENTRYPOINT ["./tgbot"]

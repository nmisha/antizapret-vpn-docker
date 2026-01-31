# ---------- build ----------
FROM golang:1.22-alpine AS build
WORKDIR /src

RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/bot ./main.go

# ---------- runtime ----------
FROM alpine:3.20
WORKDIR /app

# docker-cli нужен для /manage (docker ps/exec)
RUN apk add --no-cache ca-certificates bash docker-cli && update-ca-certificates

COPY --from=build /out/bot /app/bot

# По умолчанию ожидаем конфиги в /data (их ты смонтируешь снаружи)
ENV USERS_FILE=/data/users.json \
    DOMAINS_FILE=/data/domains.txt

# TELEGRAM_BOT_TOKEN обязателен (иначе бот завершится)
ENTRYPOINT ["/app/bot"]

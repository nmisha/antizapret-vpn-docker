FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/dns-guard ./cmd/dns-guard

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && update-ca-certificates

WORKDIR /app

COPY --from=builder /out/dns-guard ./dns-guard

ENTRYPOINT ["./dns-guard"]

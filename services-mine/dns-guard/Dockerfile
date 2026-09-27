# Cross-compile on the build host; TARGETARCH comes from buildx (--platform).
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
# No defaults: BuildKit fills them in; an empty GOARCH means the host arch.
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/dns-guard ./cmd/dns-guard

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && update-ca-certificates

WORKDIR /app

COPY --from=builder /out/dns-guard ./dns-guard

ENTRYPOINT ["./dns-guard"]

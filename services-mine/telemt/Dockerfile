ARG TELEMT_VERSION=3.5.7

# telemt release image is distroless (no shell, runs as nonroot). Wrap its static musl binary so the
# entrypoint can fix ownership of the bind-mounted config dir and drop privileges afterwards.
FROM ghcr.io/telemt/telemt:${TELEMT_VERSION} AS telemt

FROM alpine:3.22
RUN apk add --no-cache su-exec tzdata
COPY --from=telemt /app/telemt /app/telemt
COPY config.example.toml /opt/config.example.toml
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
WORKDIR /run/telemt
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD ["/app/telemt", "healthcheck", "/etc/telemt/config.toml", "--mode", "liveness"]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/etc/telemt/config.toml"]

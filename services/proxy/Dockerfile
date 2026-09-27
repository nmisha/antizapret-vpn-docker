# routes is built in services/antizapret; take it from the fork's image, not upstream.
# Build and push that image first: COPY --from pulls it from the registry.
ARG ROUTES_IMAGE=nmisha/antizapret-vpn:6.9.4-1
ARG IMAGE=ghcr.io/tarampampam/3proxy:2

FROM ${ROUTES_IMAGE} AS routes

FROM ${IMAGE} AS builder
FROM alpine:3.23

RUN apk del -U -r --no-cache iptables \
    && apk add -U --no-cache bash iptables \
    && apk cache clean

COPY --from=routes /usr/bin/routes /usr/bin/routes

COPY --from=builder /bin/portcheck /bin/3proxy /bin/lua /bin/
COPY --from=builder /entrypoint.lua /
COPY --from=builder /etc/3proxy /etc/3proxy
COPY init.sh /


CMD ["/init.sh"]

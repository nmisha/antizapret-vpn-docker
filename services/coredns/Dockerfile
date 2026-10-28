FROM golang:1.26 AS builder

ENV GOFLAGS="-buildvcs=false"
ARG COREDNS_VERSION="v1.14.4"
WORKDIR /root
RUN git clone --branch "$COREDNS_VERSION" --single-branch --depth 1 https://github.com/coredns/coredns \
    && cd coredns \
    && go mod download

ARG FINALIZER_COMMIT=ff5520b12d228cda15948c8bb247a650d916b0be
RUN cd /root/coredns \
    && sed -i '/cache:cache/a\
finalize:github.com/tmeckel/coredns-finalizer' plugin.cfg \
    && go get github.com/tmeckel/coredns-finalizer@$FINALIZER_COMMIT \
    && go generate \
    && go build

FROM ubuntu:24.04

RUN apt update -y \
    && apt install -y gettext-base dnsutils \
    && apt-get clean \
    && rm -frv /var/lib/apt/lists/*

WORKDIR /
COPY --from=coredns/coredns:latest /etc/ssl/certs/ /etc/ssl/certs/
COPY --from=builder /root/coredns/coredns /coredns

COPY . /root/

ENTRYPOINT ["/root/entrypoint.sh"]
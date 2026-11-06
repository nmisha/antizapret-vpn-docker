FROM golang:1.26 AS builder-api
WORKDIR /app
COPY ./api/go.* .
RUN go mod download
COPY ./api .
RUN go test ./... && go build -o app

FROM golang:1.26 AS builder-compose2swarm
WORKDIR /app
COPY ./compose2swarm/go.* .
RUN go mod download
COPY ./compose2swarm .
RUN go test ./... && go build -o compose2swarm

FROM golang:1.26 AS builder-routes
WORKDIR /app
COPY ./routes/go.* .
RUN go mod download
COPY ./routes .
RUN go test ./... && \
    CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -s -extldflags "-static"' -o routes

FROM ubuntu:24.04 AS builder-jool
ARG JOOL_VERSION=4.1.15
ARG JOOL_SHA256=51604bcf6a9f7fbb9080876102d05da01dc91d8c73cd5828fd73d5895643f8cd
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential pkg-config libnl-genl-3-dev curl ca-certificates
RUN curl -fL --retry 3 -o /tmp/jool.tar.gz \
      "https://github.com/NICMx/Jool/releases/download/v${JOOL_VERSION}/jool-${JOOL_VERSION}.tar.gz" && \
    echo "${JOOL_SHA256}  /tmp/jool.tar.gz" | sha256sum -c - && \
    tar -xzf /tmp/jool.tar.gz -C /tmp && \
    cd "/tmp/jool-${JOOL_VERSION}" && \
    ./configure --prefix=/opt/jool --with-xtables=no && \
    make -j2 && make install

FROM ubuntu:24.04 AS runtime-base

WORKDIR /root

ARG WARP_VERSION=2026.6.880.0

RUN <<-"EOT" bash -ex
    export APT_LISTCHANGES_FRONTEND=none DEBIAN_FRONTEND=noninteractive

    apt-get update -q
    apt-get install -qqy --no-install-suggests --no-install-recommends curl ca-certificates
    curl -fsSLo /tmp/warp.deb "https://pkg.cloudflareclient.com/pool/noble/main/c/cloudflare-warp/cloudflare-warp_${WARP_VERSION}_$(dpkg --print-architecture).deb"
    apt-get install -qqy --no-install-suggests --no-install-recommends \
        libnl-genl-3-200 \
        bsdmainutils \
        util-linux \
        iperf3 \
        dnsutils \
        gawk \
        host \
        idn \
        inetutils-ping \
        ipset \
        iptables \
        nftables \
        iproute2 \
        ipcalc \
        moreutils \
        procps \
        python3-dnslib \
        python3-maxminddb \
        sipcalc \
        vim-tiny \
        /tmp/warp.deb
    apt-get clean
    rm -frv /var/lib/apt/lists/* /tmp/warp.deb
EOT

RUN mkdir -p /usr/share/GeoIP && \
    curl -fL --retry 3 -o /usr/share/GeoIP/GeoLite2-ASN.mmdb \
        https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-ASN.mmdb

FROM runtime-base AS test-dnsmap
WORKDIR /root/antizapret
COPY ./root/antizapret/nat46.py ./root/antizapret/test_nat46.py ./root/antizapret/test_dnsmap_nat46.py ./
COPY ./root/antizapret/dnsmap.py ./root/antizapret/test_dnsmap.py ./
COPY ./root/antizapret/test_list_cache.py ./root/antizapret/list-cache.sh ./root/antizapret/download.sh ./root/antizapret/doall.sh ./
COPY ./root/antizapret/parse.sh ./
COPY ./root/antizapret/scripts ./scripts
RUN python3 -B -m unittest discover -v

FROM test-dnsmap AS runtime-files
COPY ./root /runtime/root
COPY ./init.sh ./healthcheck.sh /runtime/

RUN <<-"EOF" bash -ex
    for list in /runtime/root/antizapret/config/*-custom.txt; do rm -f $list; done
    rm -f /runtime/root/antizapret/test_*.py
EOF

FROM runtime-base

WORKDIR /root

COPY --from=builder-jool /opt/jool /opt/jool
COPY --from=builder-api /app/app /opt/api/app
COPY --from=builder-compose2swarm /app/compose2swarm /usr/bin/compose2swarm
COPY --from=builder-routes /app/routes /usr/bin/routes
COPY --from=vernette/ss-zapret2:v1.0.2 /opt/zapret2/ /opt/zapret2/
COPY --from=runtime-files /runtime/ /

RUN <<-"EOF" bash -ex
    cp -a /opt/zapret2/lua.dist /opt/zapret2/lua
    cp -a /opt/zapret2/files/fake.dist /opt/zapret2/files/fake

    echo /opt/jool/lib > /etc/ld.so.conf.d/jool.conf
    ldconfig
    ln -sf /opt/jool/bin/jool_siit /usr/bin/jool_siit

    ln -sf /root/antizapret/doall.sh /usr/bin/doall
    ln -sf /root/antizapret/dnsmap.py /usr/bin/dnsmap

    rm -frv /tmp/*
EOF

CMD ["/init.sh"]

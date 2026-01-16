FROM ubuntu:24.04

RUN apt update -y \
    && apt install -y gettext-base dnsutils \
    && apt-get clean \
    && rm -frv /var/lib/apt/lists/*

WORKDIR /
COPY --from=coredns/coredns:1.13.1 /etc/ssl/certs/ /etc/ssl/certs/
COPY --from=coredns/coredns:1.13.1 /coredns /coredns

COPY . /root/

ENTRYPOINT ["/root/entrypoint.sh"]
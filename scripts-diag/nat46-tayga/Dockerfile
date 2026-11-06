FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
    tayga iproute2 iptables python3 iputils-ping python3-aioquic python3-dnslib python3-maxminddb openssl \
    && rm -rf /var/lib/apt/lists/*

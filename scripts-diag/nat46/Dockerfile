FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends iproute2 iptables python3 libnl-genl-3-200 python3-dnslib python3-maxminddb iputils-ping && rm -rf /var/lib/apt/lists/*

RUN apt-get update && apt-get install -y --no-install-recommends python3-aioquic openssl && rm -rf /var/lib/apt/lists/*

FROM ubuntu:24.04

RUN apt update -y  \
    && apt install -y curl ca-certificates ipset iptables python3 util-linux \
    && apt-get clean \
    && rm -frv /var/lib/apt/lists/*

WORKDIR /root
COPY ./ /root
RUN chmod +x /root/*.sh

ENTRYPOINT ["/root/entrypoint.sh"]
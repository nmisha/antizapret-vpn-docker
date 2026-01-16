FROM ubuntu:24.04

RUN apt update -y  \
    && apt install -y curl ipset \
    && apt-get clean \
    && rm -frv /var/lib/apt/lists/*

WORKDIR /root
ADD ./ /root

ENTRYPOINT ["/root/entrypoint.sh"]
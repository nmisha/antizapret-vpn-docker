FROM ubuntu:24.04

RUN apt update -y && apt install -y curl ipset

WORKDIR /root
ADD ./ /root

ENTRYPOINT ["/root/entrypoint.sh"]
#!/bin/bash

docker compose config | docker run --rm -i xtrime/antizapret-vpn:5 compose2swarm > stack.rendered.yml


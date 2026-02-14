#!/bin/sh

docker compose config | docker run --rm -i xtrime/antizapret-vpn:6 compose2swarm | docker stack deploy --prune -c - antizapret

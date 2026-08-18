#!/bin/sh

# docker compose config | docker run --rm -i xtrime/antizapret-vpn:6 compose2swarm | docker stack deploy --prune -c - antizapret

docker compose --env-file compose.swarm.env config | docker run --pull always --rm -i xtrime/antizapret-vpn:6 compose2swarm | docker stack deploy --prune -c - antizapret


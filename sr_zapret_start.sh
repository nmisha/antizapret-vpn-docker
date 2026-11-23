#!/bin/bash

c=$(docker ps -q --filter name=antizapret_az-local)

docker exec \
  -e ZAPRET_CONFIG=/opt/zapret2/config/zapret.conf \
  "$c" sh /opt/zapret2/init.d/sysv/zapret2 start

# docker exec \
#   -e ZAPRET_CONFIG=/opt/zapret2/config/zapret.conf \
#   $(docker ps -q --filter=name=az-local) \
#   sh /opt/zapret2/init.d/sysv/zapret2 stop

# c=$(docker ps -q --filter name=antizapret_az-local)

# docker exec \
#   -e ZAPRET_CONFIG=/opt/zapret2/config/zapret.conf \
#   "$c" sh /opt/zapret2/init.d/sysv/zapret2 stop

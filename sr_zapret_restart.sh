#!/bin/bash

docker exec \
  -e ZAPRET_CONFIG=/opt/zapret2/config/zapret.conf \
  $(docker ps -q --filter=name=az-local) \
  sh /opt/zapret2/init.d/sysv/zapret2 restart


docker exec \
  -e ZAPRET_CONFIG=/opt/zapret2/config/zapret.conf \
  $(docker ps -q --filter=name=az-local) \
  sh /opt/zapret2/init.d/sysv/zapret2 start

  #docker exec $(docker ps -q --filter=name=az-local) sh /opt/zapret2/init.d/sysv/zapret2 start
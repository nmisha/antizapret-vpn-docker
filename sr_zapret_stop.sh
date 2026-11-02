#!/bin/bash


docker exec \
  -e ZAPRET_CONFIG=/opt/zapret2/config/zapret.conf \
  $(docker ps -q --filter=name=az-local) \
  sh /opt/zapret2/init.d/sysv/zapret2 stop
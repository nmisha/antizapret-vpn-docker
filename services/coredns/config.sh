#!/usr/bin/env bash

export AZ_LOCAL_HOST=$(dig +short az-local | head -n1)
export AZ_WORLD_HOST=$(dig +short az-world | head -n1)
export DNS_HOST=$(dig +short adguard | head -n1)

envsubst < /root/Corefile.template > /Corefile
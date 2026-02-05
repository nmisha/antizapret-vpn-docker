#!/usr/bin/env bash

# resolve domain address to ip address
function resolve () {
    # $1 domain/ip address, $2 fallback ip address
    res="$(getent hosts "$1" | head -n1 | awk '{print $1}')"
    if [[ "$res" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        echo "$res"
    else
        echo "$2"
    fi
}

export AZ_LOCAL_HOST=$(resolve 'az-local' '169.254.0.1')
export AZ_WORLD_HOST=$(resolve 'az-world' '169.254.0.2')
export DNS_HOST=$(resolve 'adguard' '169.254.0.3')

envsubst < /root/Corefile.template > /Corefile
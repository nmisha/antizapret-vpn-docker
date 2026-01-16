#!/usr/bin/env bash

# resolve domain address to ip address
function resolve () {
    # $1 domain/ip address, $2 fallback ip address
    res="$(dig +short $1 | head -n1)"
    if [[ "$res" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        echo "$res"
    else
        echo "$2"
    fi
}

export AZ_LOCAL_HOST=$(resolve 'az-local' '')
export AZ_WORLD_HOST=$(resolve 'az-world' '')
export DNS_HOST=$(resolve 'adguard' '')

envsubst < /root/Corefile.template > /Corefile
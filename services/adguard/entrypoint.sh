#!/usr/bin/env bash

cp -n /root/AdGuardHome.yaml /opt/adguardhome/conf/AdGuardHome.yaml

CONFIG_FILES="/root/antizapret/result/* /root/antizapret/config/custom/*"
cat $CONFIG_FILES 2>/dev/null | md5sum | cut -d' ' -f1 > /.config_md5

ADGUARDHOME_PORT=${ADGUARDHOME_PORT:-"3000"}
ADGUARDHOME_USERNAME=${ADGUARDHOME_USERNAME:-"admin"}
if [[ -n $ADGUARDHOME_PASSWORD ]]; then
    ADGUARDHOME_PASSWORD_HASH=$(htpasswd -B -C 10 -n -b "$ADGUARDHOME_USERNAME" "$ADGUARDHOME_PASSWORD")
    ADGUARDHOME_PASSWORD_HASH=${ADGUARDHOME_PASSWORD_HASH#*:}
fi


/root/routes.sh &

function resolve () {
    # $1 domain/ip address, $2 fallback ip address
    res="$(dig +short $1 | head -n1)"
    if [[ "$res" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        echo "$res"
    else
        echo "$2"
    fi
}

AZ_LOCAL_HOST=$(resolve az-local '169.0.0.1')
AZ_WORLD_HOST=$(resolve az-world '169.0.0.2')
COREDNS_HOST=$(resolve coredns '169.0.0.3')

yq -i '
    .http.address="0.0.0.0:'$ADGUARDHOME_PORT'" |
    .users[0].name="'$ADGUARDHOME_USERNAME'" |
    .users[0].password="'$ADGUARDHOME_PASSWORD_HASH'" |
    (.clients.persistent[] | select(.name == "az-local") | .ids) = ["'$AZ_LOCAL_HOST'"] |
    (.clients.persistent[] | select(.name == "az-world") | .ids) = ["'$AZ_WORLD_HOST'"] |
    (.clients.persistent[] | select(.name == "coredns") | .ids) = ["'$COREDNS_HOST'"]
    ' /opt/adguardhome/conf/AdGuardHome.yaml



exec /opt/adguardhome/AdGuardHome "$@"
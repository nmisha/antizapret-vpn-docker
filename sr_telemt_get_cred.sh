#!/bin/bash

cd /root/antizapret6-swarm

sudo python3 - <<'PY'
import tomllib
from urllib.parse import urlencode

with open("config-mine/telemt/config.toml", "rb") as f:
    c = tomllib.load(f)

user = "admin"
secret = (
    "ee"
    + c["access"]["users"][user]
    + c["censorship"]["tls_domain"].encode().hex()
)
links = c["general"]["links"]

print("Сервер:", links["public_host"])
print("Порт:", links["public_port"])
print("Секрет:", secret)
print("tg://proxy?" + urlencode({
    "server": links["public_host"],
    "port": links["public_port"],
    "secret": secret,
}))
PY
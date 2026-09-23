#!/bin/sh
set -eu
cd "$(dirname "$0")"
exec flock -x /run/antizapret-firewall.lock python3 firewall.py "${1:-apply}"

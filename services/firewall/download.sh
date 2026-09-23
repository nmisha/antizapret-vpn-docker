#!/bin/bash
set -eu
# Failed/partial downloads never overwrite the previous file.
tmp=$(mktemp "${2}.XXXXXX")
trap 'rm -f "$tmp"' EXIT
curl --max-time 60 --fail --silent --show-error --location "$1" -o "$tmp"
[ -s "$tmp" ]
mv -f "$tmp" "$2"

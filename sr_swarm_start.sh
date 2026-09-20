#!/bin/sh

set -eu
umask 077
# Stop at the first failed stage. Rendered files may contain secrets.
work_dir=$(mktemp -d)
trap 'rm -rf -- "$work_dir"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker compose --env-file compose.swarm.env config > "$work_dir/compose.yml"
docker run --pull always --rm -i xtrime/antizapret-vpn:6 compose2swarm \
  < "$work_dir/compose.yml" > "$work_dir/stack.yml"
docker stack config -c "$work_dir/stack.yml" > /dev/null
docker stack deploy --prune -c "$work_dir/stack.yml" antizapret

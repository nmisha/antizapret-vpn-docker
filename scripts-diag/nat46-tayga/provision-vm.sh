#!/bin/bash
# Explicitly provision a disposable Ubuntu VM only.
set -euo pipefail
[ "${1:-}" = --disposable-vm ] || {
    echo 'Usage: bash provision-vm.sh --disposable-vm (inside the test VM only)' >&2
    exit 2
}
sudo apt-get update
sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y docker.io
sudo systemctl enable --now docker
sudo touch /etc/antizapret-nat46-test-vm

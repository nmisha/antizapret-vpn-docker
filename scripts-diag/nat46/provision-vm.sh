#!/bin/bash
# Run only in the disposable Ubuntu test VM, never on production hosts.
set -euo pipefail
sudo touch /etc/antizapret-nat46-test-vm
sudo apt-get update
sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y \
  docker.io dkms build-essential pkg-config curl ca-certificates \
  "linux-headers-$(uname -r)" libnl-genl-3-dev libxtables-dev
mkdir -p /tmp/nat46-jool-build
cd /tmp/nat46-jool-build
curl -fL --retry 3 -o jool.tar.gz https://github.com/NICMx/Jool/releases/download/v4.1.15/jool-4.1.15.tar.gz
echo '51604bcf6a9f7fbb9080876102d05da01dc91d8c73cd5828fd73d5895643f8cd  jool.tar.gz' | sha256sum -c -
tar -xzf jool.tar.gz
if ! dkms status -m jool -v 4.1.15 -k "$(uname -r)" | grep 'installed' >/dev/null; then
    sudo dkms install ./jool-4.1.15/
fi
cd jool-4.1.15
./configure
make -j2
sudo make install
sudo ldconfig
sudo modprobe jool_siit
printf '%s\n' jool_siit | sudo tee /etc/modules-load.d/jool-siit.conf
sudo systemctl start docker
uname -r
modinfo -F version jool_siit
sudo jool_siit instance display

#!/usr/bin/env python3
"""Run on world after telemt has created its config; requires Python 3.11+."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tomllib

IMAGE = "ghcr.io/amirotin/telemt_panel:1.0.0-rc.2"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--domain", required=True)
    args = parser.parse_args()
    root = args.root.resolve()
    source = root / "config-mine/telemt/config.toml"
    base = root / "config-mine/telemt-panel"
    target = base / "config/config.toml"
    if target.exists():
        raise SystemExit(f"Already configured: {target}; existing settings/password preserved.")
    with source.open("rb") as stream:
        token = tomllib.load(stream)["server"]["api"]["auth_header"]
    if not token:
        raise SystemExit("telemt API auth_header must not be empty")
    q = json.dumps
    config = f'''listen = "0.0.0.0:8080"
public_url = {q("https://" + args.domain)}
trusted_proxies = ["10.44.42.0/24"]
data_dir = "/var/lib/telemt-panel"

[tls]
mode = "http"

[telemt]
url = "http://telemt:9091"
auth_header = {q(token)}

[auth]
disabled = true

[store]
driver = "sqlite"
path = "/var/lib/telemt-panel/panel.db"

[subpage]
enabled = false

[host]
service_manager = "none"

[privileges]
mode = "manual"
'''
    tomllib.loads(config)
    os.umask(0o077)
    (base / "config").mkdir(parents=True, exist_ok=True, mode=0o700)
    (base / "data").mkdir(parents=True, exist_ok=True, mode=0o700)
    with target.open("x") as stream:
        stream.write(config)
    subprocess.run([
        "docker", "run", "--rm", "--mount",
        f"type=bind,source={base / 'config'},target=/etc/telemt-panel,readonly",
        IMAGE, "config", "check", "--config", "/etc/telemt-panel/config.toml",
    ], check=True)
    print(f"Prepared {target}. Authentication: Authelia via Caddy; private network required.")


if __name__ == "__main__":
    main()

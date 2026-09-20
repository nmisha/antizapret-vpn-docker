#!/usr/bin/env python3
"""Run on world after telemt has created its config; requires Python 3.11+."""
import argparse
import json
import os
from pathlib import Path
import secrets
import subprocess
import tomllib

IMAGE = "ghcr.io/amirotin/telemt_panel:1.0.0-rc.2"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--domain", default="telemt.marina.2bd.net")
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
    password = secrets.token_urlsafe(24)
    result = subprocess.run(
        ["docker", "run", "--rm", "-i", IMAGE, "hash-password"],
        input=password + "\n", text=True, capture_output=True, check=True,
    )
    password_hash = result.stdout.strip()
    if not password_hash.startswith(("$2a$", "$2b$", "$2y$")):
        raise SystemExit("Panel did not return a bcrypt hash")
    q = json.dumps
    config = f'''listen = "0.0.0.0:8080"
public_url = {q("https://" + args.domain)}
trusted_proxies = ["10.200.0.0/24"]
data_dir = "/var/lib/telemt-panel"

[tls]
mode = "http"

[telemt]
url = "http://telemt:9091"
auth_header = {q(token)}

[auth]
username = "admin"
password_hash = {q(password_hash)}

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
    with (base / "initial-login.txt").open("x") as stream:
        stream.write(f"URL: https://{args.domain}\nUsername: admin\nPassword: {password}\n")
    subprocess.run([
        "docker", "run", "--rm", "--mount",
        f"type=bind,source={base / 'config'},target=/etc/telemt-panel,readonly",
        IMAGE, "config", "check", "--config", "/etc/telemt-panel/config.toml",
    ], check=True)
    print(f"Prepared {target}. Initial credentials: {base / 'initial-login.txt'}")


if __name__ == "__main__":
    main()

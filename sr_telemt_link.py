#!/usr/bin/env python3
"""Print a user's Fake TLS proxy link from the local telemt config (Python 3.11+)."""
import argparse
from pathlib import Path
import tomllib
from urllib.parse import urlencode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("user", nargs="?", default="admin")
    parser.add_argument(
        "--config", type=Path,
        default=Path(__file__).resolve().parent / "config-mine/telemt/config.toml",
    )
    args = parser.parse_args()
    try:
        with args.config.open("rb") as stream:
            config = tomllib.load(stream)
        secret = (
            "ee" + config["access"]["users"][args.user]
            + config["censorship"]["tls_domain"].encode().hex()
        )
        links = config["general"]["links"]
        params = {
            "server": links["public_host"],
            "port": links["public_port"],
            "secret": secret,
        }
    except (OSError, KeyError, TypeError, tomllib.TOMLDecodeError) as exc:
        parser.exit(1, f"Cannot read proxy settings: {exc}\n")
    print("Сервер:", params["server"])
    print("Порт:", params["port"])
    print("Секрет:", secret)
    print("tg://proxy?" + urlencode(params))


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Compare TLS profiles without running a browser.

Install the optional dependency in an isolated environment:
  python3 -m venv /tmp/youtube-tls-lab-venv
  /tmp/youtube-tls-lab-venv/bin/pip install curl_cffi==0.17.0
  /tmp/youtube-tls-lab-venv/bin/python scripts/compare-tls-profiles.py

All requests use TCP, verify certificates, bypass environment proxies, and pin
the target hostname to one IPv4 address. New sessions prevent connection reuse.
This is a diagnostic client, not a transparent gateway for other applications.
"""

import argparse
from collections import Counter
import json
from pathlib import Path
import socket
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="www.youtube.com")
    parser.add_argument("--ip", help="IPv4 destination; default: first DNS answer")
    parser.add_argument("--rounds", type=int, default=3)
    parser.add_argument("--timeout", type=float, default=6)
    parser.add_argument("--output", type=Path, help="Save JSON results")
    args = parser.parse_args()
    if args.rounds < 1 or args.timeout <= 0:
        parser.error("--rounds and --timeout must be positive")
    if not args.host or any(char in args.host for char in "/:@?#\\ \t\n"):
        parser.error("--host must be a hostname")
    try:
        import curl_cffi
        from curl_cffi import CurlHttpVersion, CurlOpt, requests
    except ImportError:
        parser.error("Install curl_cffi==0.17.0 in a virtual environment; see --help")
    try:
        ip = args.ip or socket.getaddrinfo(
            args.host, 443, socket.AF_INET, socket.SOCK_STREAM,
        )[0][4][0]
        socket.inet_pton(socket.AF_INET, ip)
    except (OSError, IndexError) as error:
        parser.error(str(error))
    cases = [
        ("boringssl_default", None, {}),
        ("chrome152", "chrome152", {}),
        ("chrome124", "chrome124", {}),
        ("chrome110", "chrome110", {}),
        ("firefox155", "firefox155", {}),
        ("chrome152_no_pq", "chrome152", {
            CurlOpt.SSL_EC_CURVES: b"X25519:P-256:P-384",
        }),
        ("chrome152_no_ech", "chrome152", {CurlOpt.ECH: b"false"}),
        ("chrome152_no_grease", "chrome152", {CurlOpt.TLS_GREASE: 0}),
    ]
    url = f"https://{args.host}/"
    print(f"curl_cffi={curl_cffi.__version__}; {args.host}:443 -> {ip}; TCP only", flush=True)
    results = []
    for iteration in range(1, args.rounds + 1):
        # Reverse alternate rounds to reduce a fixed ordering bias.
        for name, profile, extra in (cases if iteration % 2 else reversed(cases)):
            start = time.monotonic()
            result = {"round": iteration, "case": name}
            options = {
                CurlOpt.RESOLVE: [f"{args.host}:443:{ip}".encode()],
                **extra,
            }
            try:
                with requests.Session(trust_env=False, curl_options=options) as session:
                    response = session.get(
                        url, impersonate=profile,
                        http_version=CurlHttpVersion.V2TLS,
                        timeout=(args.timeout, args.timeout),
                        allow_redirects=False,
                    )
                result.update(status=response.status_code, ip=response.primary_ip,
                              http_version={1: "HTTP/1.0", 2: "HTTP/1.1", 3: "HTTP/2",
                                            30: "HTTP/3"}.get(int(response.http_version),
                                                             str(response.http_version)),
                              bytes=len(response.content))
            except Exception as error:
                result["error"] = str(error)
            result["seconds"] = round(time.monotonic() - start, 3)
            results.append(result)
            print(json.dumps(result, ensure_ascii=False), flush=True)
        # Use the system curl as a control in each round.
        control = subprocess.run([
            "curl", "--noproxy", "*", "--resolve", f"{args.host}:443:{ip}",
            "--silent", "--show-error", "--output", "/dev/null",
            "--connect-timeout", str(args.timeout), "--max-time", str(args.timeout * 2),
            "--write-out", "%{http_code} %{remote_ip} %{http_version}", url,
        ], capture_output=True, text=True, timeout=args.timeout * 2 + 3)
        result = {"round": iteration, "case": "system_curl",
                  "exit_code": control.returncode, "result": control.stdout,
                  "stderr": control.stderr.strip()}
        results.append(result)
        print(json.dumps(result, ensure_ascii=False), flush=True)
    counts = Counter(row["case"] for row in results if row.get("status") == 200)
    report = {"host": args.host, "ip": ip, "curl_cffi_version": curl_cffi.__version__,
              "rounds": args.rounds, "http_200_counts": dict(counts), "results": results}
    if args.output:
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print("HTTP 200 counts:", dict(counts))


if __name__ == "__main__":
    main()

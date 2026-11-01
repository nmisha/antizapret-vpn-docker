#!/usr/bin/env python3
"""Compare real Chrome and curl over TCP to the same HTTPS endpoint.

Uses only the Python standard library. Chrome runs with a new temporary profile;
the existing browser and its settings are not changed. Diagnostic artifacts may
contain response headers and browsing data from the test profile.
"""

import argparse
import concurrent.futures
import json
import os
from pathlib import Path
import re
import shutil
import signal
import socket
import subprocess
import tempfile
from urllib.parse import urlsplit


def run(command, timeout):
    process = subprocess.Popen(
        command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        text=True, start_new_session=True,
    )
    try:
        stdout, stderr = process.communicate(timeout=timeout)
        return process.returncode, stdout, stderr
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            stdout, stderr = process.communicate(timeout=3)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            stdout, stderr = process.communicate()
        return 124, stdout, stderr


def chrome_summary(path, host):
    if not path.exists():
        return {"error": "Chrome did not create a NetLog"}
    try:
        log = json.loads(path.read_text())
    except (ValueError, OSError) as error:
        return {"error": str(error)}
    names = {value: name for name, value in log["constants"]["logEventTypes"].items()}
    events = log["events"]
    sessions = {
        event["source"]["id"]
        for event in events
        if names[event["type"]] == "HTTP2_SESSION"
        and event.get("params", {}).get("host") == host + ":443"
    }
    sockets = set()
    statuses = []
    for event in events:
        if event["source"]["id"] not in sessions:
            continue
        params = event.get("params", {})
        name = names[event["type"]]
        if name == "HTTP2_SESSION_INITIALIZED":
            sockets.add(params["source_dependency"]["id"])
        if name == "HTTP2_SESSION_RECV_HEADERS":
            statuses.extend(
                header for header in params.get("headers", [])
                if header.startswith(":status:")
            )
    tls = []
    addresses = []
    for event in events:
        if event["source"]["id"] not in sockets:
            continue
        params = event.get("params", {})
        name = names[event["type"]]
        if name == "TCP_CONNECT":
            addresses.extend(params.get("address_list", []))
        if name == "SSL_CONNECT" and event["phase"] == 2:
            tls.append({key: params[key] for key in (
                "version", "next_proto", "encrypted_client_hello",
                "key_exchange_group", "cipher_suite", "net_error",
            ) if key in params})
    return {
        "successful_http2": bool(statuses),
        "http_statuses": sorted(set(statuses)),
        "addresses": sorted(set(addresses)),
        "tls": tls,
        "note": "Only target-host HTTP/2 sessions are summarized; see NetLog for failures and redirects.",
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="https://www.youtube.com/")
    parser.add_argument("--ip", help="Pin both clients to this IPv4 address; default: first DNS answer")
    parser.add_argument("--chrome", help="Path to the installed Chrome executable")
    parser.add_argument("--timeout", type=int, default=20, help="Chrome test duration in seconds (default: 20)")
    args = parser.parse_args()
    url = urlsplit(args.url)
    if url.scheme != "https" or not url.hostname or url.port not in (None, 443):
        parser.error("Use an HTTPS URL on port 443")
    if args.timeout < 5:
        parser.error("--timeout must be at least 5")
    chrome = args.chrome or next((str(path) for path in (
        Path("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
        Path(shutil.which("google-chrome") or "/nonexistent"),
        Path(shutil.which("chromium") or "/nonexistent"),
    ) if path.is_file()), None)
    if not chrome or not shutil.which("curl"):
        parser.error("Both Chrome and curl must be installed")
    try:
        answers = sorted({item[4][0] for item in socket.getaddrinfo(
            url.hostname, 443, socket.AF_INET, socket.SOCK_STREAM,
        )})
        ip = args.ip or answers[0]
        socket.inet_pton(socket.AF_INET, ip)
    except (OSError, IndexError) as error:
        parser.error(str(error))
    root = Path(tempfile.mkdtemp(prefix="chrome-curl-compare-"))
    print(f"Target: {url.hostname}:443 -> {ip}", flush=True)
    print(f"DNS A: {', '.join(answers)}", flush=True)
    print(f"Artifacts: {root}", flush=True)
    chrome_command = [
        chrome, "--headless=new", "--user-data-dir=" + str(root / "profile"),
        "--no-first-run", "--no-default-browser-check", "--disable-quic",
        "--disable-background-networking",
        f"--host-resolver-rules=MAP {url.hostname} {ip}",
        "--log-net-log=" + str(root / "chrome-netlog.json"),
        "--dump-dom", f"--timeout={args.timeout * 1000}", args.url,
    ]
    curl_command = [
        "curl", "--noproxy", "*", "--resolve", f"{url.hostname}:443:{ip}",
        "--verbose", "--connect-timeout", "6", "--max-time", str(args.timeout),
        "--output", os.devnull, "--write-out", "%{json}", args.url,
    ]
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        chrome_future = pool.submit(run, chrome_command, args.timeout + 10)
        curl_future = pool.submit(run, curl_command, args.timeout + 5)
        chrome_code, dom, chrome_err = chrome_future.result()
        curl_code, curl_out, curl_err = curl_future.result()
    (root / "chrome-dom.html").write_text(dom)
    (root / "chrome-stderr.txt").write_text(chrome_err)
    (root / "curl-verbose.txt").write_text(curl_err)
    (root / "curl-result.json").write_text(curl_out)
    try:
        metrics = json.loads(curl_out)
    except ValueError:
        metrics = {}
    summary = {
        "target": {"url": args.url, "ip": ip, "dns_a": answers, "quic": False},
        "chrome": {
            "exit_code": chrome_code,
            "title": re.findall(r"<title>(.*?)</title>", dom, re.S)[:1],
            "dom_bytes": len(dom.encode()),
            **chrome_summary(root / "chrome-netlog.json", url.hostname),
        },
        "curl": {
            "exit_code": curl_code,
            **{key: metrics.get(key) for key in (
                "http_code", "remote_ip", "http_version", "ssl_verify_result",
                "time_connect", "time_appconnect", "time_total", "errormsg",
            )},
        },
    }
    formatted = json.dumps(summary, indent=2, ensure_ascii=False)
    (root / "summary.json").write_text(formatted + "\n")
    print(formatted)
    return 0 if summary["chrome"].get("successful_http2") else 1


if __name__ == "__main__":
    raise SystemExit(main())

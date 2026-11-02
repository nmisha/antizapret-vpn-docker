#!/usr/bin/env python3
"""Test unchanged system curl through a local TCP relay with ClientHello splitting.

The relay binds only to loopback, forwards to one IPv4 address, and never
terminates TLS. It splits TCP writes inside the hostname, leaving all TLS
bytes unchanged. Packet boundaries are not guaranteed by TCP; this is a
diagnostic experiment, not a production proxy. Requires Python 3 and curl.
"""

import argparse
from collections import Counter
import json
from pathlib import Path
import select
import socket
import subprocess
import threading
import time


def probe(host, ip, split, offset, delay, timeout):
    listener = socket.socket()
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)
    port = listener.getsockname()[1]
    stop = threading.Event()
    details = {}

    def relay():
        try:
            listener.settimeout(timeout + 2)
            with listener.accept()[0] as client, socket.create_connection(
                (ip, 443), timeout=timeout,
            ) as server:
                server.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
                client.settimeout(timeout)
                # Assemble the first TLS record before splitting TCP writes.
                data = bytearray()
                while len(data) < 5:
                    chunk = client.recv(5 - len(data))
                    if not chunk:
                        return
                    data.extend(chunk)
                size = 5 + int.from_bytes(data[3:5], "big")
                while len(data) < size:
                    chunk = client.recv(size - len(data))
                    if not chunk:
                        return
                    data.extend(chunk)
                position = data.find(host.encode("ascii"))
                details["clienthello_record_bytes"] = len(data)
                details["hostname_offset"] = position
                if split:
                    if position < 0 or not 0 < offset < len(host):
                        raise ValueError("Cannot split inside the requested hostname")
                    cut = position + offset
                    details["tcp_write_sizes"] = [cut, len(data) - cut]
                    server.sendall(data[:cut])
                    time.sleep(delay)
                    server.sendall(data[cut:])
                else:
                    details["tcp_write_sizes"] = [len(data)]
                    server.sendall(data)
                while not stop.is_set():
                    ready, _, _ = select.select([client, server], [], [], 0.2)
                    for source in ready:
                        chunk = source.recv(65536)
                        if not chunk:
                            return
                        (server if source is client else client).sendall(chunk)
        except Exception as error:
            details["relay_error"] = str(error)
        finally:
            listener.close()

    thread = threading.Thread(target=relay, daemon=True)
    thread.start()
    try:
        result = subprocess.run([
            "curl", "--noproxy", "*", "--connect-to",
            f"{host}:443:127.0.0.1:{port}", "--silent", "--show-error",
            "--output", "/dev/null", "--connect-timeout", str(timeout),
            "--max-time", str(timeout * 2), "--write-out", "%{json}",
            f"https://{host}/",
        ], capture_output=True, text=True, timeout=timeout * 2 + 3)
        try:
            metrics = json.loads(result.stdout)
        except ValueError:
            metrics = {}
        return {
            "mode": "split_sni" if split else "baseline",
            "exit_code": result.returncode,
            **{key: metrics.get(key) for key in (
                "http_code", "http_version", "ssl_verify_result", "time_total",
            )},
            "stderr": result.stderr.strip(), "relay": details,
        }
    finally:
        stop.set()
        thread.join(timeout=1)
        listener.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="www.youtube.com")
    parser.add_argument("--ip")
    parser.add_argument("--rounds", type=int, default=3)
    parser.add_argument("--offset", type=int, default=4, help="Split after this many hostname bytes")
    parser.add_argument("--delay", type=float, default=0.04, help="Seconds between the two TCP writes")
    parser.add_argument("--timeout", type=float, default=6)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    if not args.host or any(char in args.host for char in "/:@?#\\ \t\n"):
        parser.error("--host must be an ASCII hostname")
    if args.rounds < 1 or args.timeout <= 0 or args.delay < 0:
        parser.error("Invalid rounds, timeout, or delay")
    if not 0 < args.offset < len(args.host):
        parser.error("--offset must fall inside the hostname")
    try:
        args.host.encode("ascii")
        ip = args.ip or socket.getaddrinfo(
            args.host, 443, socket.AF_INET, socket.SOCK_STREAM,
        )[0][4][0]
        socket.inet_pton(socket.AF_INET, ip)
    except (OSError, IndexError, UnicodeError) as error:
        parser.error(str(error))
    print(f"{args.host}:443 -> {ip}; TLS verification enabled; TCP only", flush=True)
    results = []
    for iteration in range(1, args.rounds + 1):
        for split in ([False, True] if iteration % 2 else [True, False]):
            result = probe(args.host, ip, split, args.offset, args.delay, args.timeout)
            result["round"] = iteration
            results.append(result)
            print(json.dumps(result), flush=True)
    counts = Counter(row["mode"] for row in results if row.get("http_code") == 200)
    report = {"host": args.host, "ip": ip, "delay": args.delay, "offset": args.offset,
              "rounds": args.rounds, "http_200_counts": dict(counts), "results": results}
    if args.output:
        args.output.write_text(json.dumps(report, indent=2) + "\n")
    print("HTTP 200 counts:", dict(counts))


if __name__ == "__main__":
    main()

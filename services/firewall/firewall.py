#!/usr/bin/env python3
"""Own only AZ-VPN-FILTER and tagged hooks; never flush host firewall."""
import ipaddress
import os
from pathlib import Path
import re
import subprocess
import sys

CHAIN = "AZ-VPN-FILTER"
TAG = "antizapret-firewall"

def run(*args, data=None, check=True):
    return subprocess.run(args, input=data, text=True, capture_output=True, check=check)

def entries(path, version):
    values = []
    for line in Path(path).read_text().splitlines():
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        net = ipaddress.ip_network(line, strict=False)
        if net.version != version:
            raise ValueError(f"Wrong IP family in {path}: {line}")
        values.append(str(net))
    if not values:
        raise ValueError(f"Empty blocklist: {path}; keeping existing rules")
    return sorted(set(values))

def exceptions():
    path = os.environ.get("EXCEPTIONS_FILE", "")
    result = {4: [], 6: []}
    if not path:
        return result
    for number, line in enumerate(Path(path).read_text().splitlines(), 1):
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        interface, address, protocol, ports = line.split()
        if not re.fullmatch(r"[a-zA-Z0-9_.:-]{1,15}", interface):
            raise ValueError(f"Invalid interface on line {number}")
        address = ipaddress.ip_address(address)
        if protocol not in ("tcp", "udp"):
            raise ValueError(f"Invalid protocol on line {number}")
        for port in ports.split(","):
            if not port.isdecimal() or not 1 <= int(port) <= 65535:
                raise ValueError(f"Invalid port on line {number}")
            result[address.version].append(
                f"-A {CHAIN} -i {interface} -p {protocol} -m conntrack "
                f"--ctdir ORIGINAL --ctorigdst {address} --ctorigdstport {int(port)} -j RETURN")
    return result

def tool(version):
    return "iptables" if version == 4 else "ip6tables"

def hook():
    return ["-m", "comment", "--comment", TAG, "-j", CHAIN]

def clear():
    for version in (4, 6):
        cmd = tool(version)
        while run(cmd, "-w", "10", "-C", "DOCKER-USER", *hook(), check=False).returncode == 0:
            run(cmd, "-w", "10", "-D", "DOCKER-USER", *hook())
        run(cmd, "-w", "10", "-F", CHAIN, check=False)
        run(cmd, "-w", "10", "-X", CHAIN, check=False)
        for suffix in ("", "_next"):
            run("ipset", "destroy", f"az_firewall_v{version}{suffix}", check=False)

def apply():
    # Validate BOTH lists and all exceptions before changing any live rule.
    lists = {v: entries(os.environ[f"V{v}_FILE"], v) for v in (4, 6)}
    allow = exceptions()
    for version in (4, 6):
        run(tool(version), "-w", "10", "-nL", "DOCKER-USER")
    # Build both staging sets before swapping either live set.
    for version in (4, 6):
        name = f"az_firewall_v{version}"
        family = "inet" if version == 4 else "inet6"
        for target in (name, name + "_next"):
            run("ipset", "create", target, "hash:net", "family", family,
                "hashsize", "4096", "maxelem", "200000", "-exist")
        run("ipset", "flush", name + "_next")
        run("ipset", "restore", data="".join(f"add {name}_next {net}\n" for net in lists[version]))
    for version in (4, 6):
        name = f"az_firewall_v{version}"
        run("ipset", "swap", name + "_next", name)
    for version in (4, 6):
        cmd = tool(version)
        name = f"az_firewall_v{version}"
        # One per-family transaction replaces only our chain, not Docker/UFW.
        rules = ["*filter", f":{CHAIN} - [0:0]", f"-F {CHAIN}",
                 f"-A {CHAIN} -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN"]
        rules += allow[version]
        rules += [f"-A {CHAIN} -m set --match-set {name} src -j DROP", f"-A {CHAIN} -j RETURN", "COMMIT", ""]
        run(cmd + "-restore", "--wait", "10", "--noflush", data="\n".join(rules))
        if run(cmd, "-w", "10", "-C", "DOCKER-USER", *hook(), check=False).returncode:
            run(cmd, "-w", "10", "-I", "DOCKER-USER", "1", *hook())
        # Migrate the exact old DROP; do not delete untagged ESTABLISHED rules.
        legacy = ["-m", "set", "--match-set", name, "src", "-j", "DROP"]
        while run(cmd, "-w", "10", "-C", "DOCKER-USER", *legacy, check=False).returncode == 0:
            run(cmd, "-w", "10", "-D", "DOCKER-USER", *legacy)
    print("Firewall lists and exceptions applied", flush=True)

if __name__ == "__main__":
    try:
        action = sys.argv[1] if len(sys.argv) > 1 else "apply"
        if action == "clear": clear()
        elif action == "apply": apply()
        else: raise ValueError("Usage: block.sh [apply|clear]")
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"Firewall update failed: {error}", file=sys.stderr)
        if isinstance(error, subprocess.CalledProcessError): print(error.stderr, file=sys.stderr)
        sys.exit(1)

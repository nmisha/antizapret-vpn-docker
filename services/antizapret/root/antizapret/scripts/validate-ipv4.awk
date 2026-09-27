# Usage: awk [-v mode=filter|exact|regex] [-v list=NAME] -f validate-ipv4.awk FILE
# mode=filter (default): print valid IPv4 addresses/CIDRs, report the rest to
#   stderr. One bad line (IPv6, typo) must not block publishing every list.
# mode=exact / mode=regex: split an exclude list into lines that are plain
#   IPv4 addresses/CIDRs (compared exactly) and everything else (EREs).

function valid_ipv4(value,    parts, octets, count, i) {
    if (value !~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+(\/[0-9]+)?$/) return 0
    count = split(value, parts, "/")
    if (count == 2 && (parts[2] + 0) > 32) return 0
    split(parts[1], octets, ".")
    for (i = 1; i <= 4; i++) {
        if ((octets[i] + 0) > 255) return 0
    }
    return 1
}

{
    ok = valid_ipv4($0)
    if (mode == "exact") {
        if (ok) print
    } else if (mode == "regex") {
        if (!ok) print
    } else if (ok) {
        print
    } else {
        printf "Skipping invalid IPv4 entry in %s list: %s\n", (list ? list : FILENAME), $0 > "/dev/stderr"
    }
}

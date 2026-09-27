# Usage: awk [-v whole_line=1] -f sanitize-lists.awk FILE
# Removes comments, surrounding whitespace and empty lines.
# By default only the first field is kept (IP/CIDR lists). ASN lists need
# whole_line=1: organization names and /regex/ rules may contain spaces.
BEGIN {
    RS="\n|\r";
    IGNORECASE=1;
}

{
    line = whole_line ? $0 : $1
    sub(/#.*$/, "", line)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
    if (line == "") next
    print line
}

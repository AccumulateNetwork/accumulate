#!/bin/bash

# Stop immediately on error
set -e

# section <name> - Print a section header
function section {
  echo -e '\033[1m'"$1"'\033[0m'
}

# die <message> - Print an error message and exit
function die {
    echo -e '\033[1;31m'"$@"'\033[0m'
    exit 1
}

# success - Print 'success' in bold green text
function success {
    echo -e '\033[1;32m'Success'\033[0m'
    echo
}

section "Test minor block API omit-tx on BVN0"
accumulate blocks minor acc://bvn-bvn0 10 5 omit >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  die "minor block result set too small"
fi

section "Test minor block API omit-tx on DN"
accumulate blocks minor acc://dn 10 5 omit >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  die "minor block result set too small"
fi

section "Test minor block API count-only on BVN0"
accumulate blocks minor acc://bvn-bvn0 10 5 countOnly >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  die "minor block result set too small"
fi

section "Test minor block API count-only on DN"
accumulate blocks minor acc://dn 10 5 countOnly >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  die "minor block result set too small"
fi

section "Test minor block API expand on BVN0"
accumulate blocks minor acc://bvn-bvn0 10 5 expand true >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  die "minor block result set too small"
fi

section "Test minor block API expand on DN"
accumulate blocks minor acc://dn 10 5 expand true >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  die "minor block result set too small"
fi

success
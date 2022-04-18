#!/bin/bash

# Stop immediately on error
set -e

# section <name> - Print a section header
function section {
  echo -e '\033[1m'"$1"'\033[0m'
}

section "Test minor block API omit-tx on BVN0"
accumulate blocks minor acc://bvn-bvn0 10 5 omit >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
echo File size is ${FILESIZE}

section "Test minor block API omit-tx on DN"
accumulate blocks minor acc://dn 10 5 omit >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
echo File size is ${FILESIZE}

section "Test minor block API count-only on BVN0"
accumulate blocks minor acc://bvn-bvn0 10 5 countOnly >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
echo File size is ${FILESIZE}

section "Test minor block API count-only on DN"
accumulate blocks minor acc://dn 10 5 countOnly >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
echo File size is ${FILESIZE}

section "Test minor block API expand on BVN0"
accumulate blocks minor acc://bvn-bvn0 10 5 expand true >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
echo File size is ${FILESIZE}

section "Test minor block API expand on DN"
accumulate blocks minor acc://dn 10 5 expand true >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
echo File size is ${FILESIZE}

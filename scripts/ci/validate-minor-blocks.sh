#!/bin/bash

# Stop immediately on error
set -e

SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
source ${SCRIPT_DIR}/validate-commons.sh

# die <message> - Print an error message and exit
function dieMinor {
  echo -e '\033[1;31m'"$@"'\033[0m'
  echo the output was:
  cat /tmp/minor.txt
  exit 1
}

section "Test minor block API omit-tx on BVN0"
accumulate --use-unencrypted-wallet blocks minor acc://bvn-bvn0 10 5 omit >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieMinor "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API omit-tx on DN"
accumulate --use-unencrypted-wallet blocks minor acc://dn 10 5 omit >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieMinor "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API count-only on BVN0"
accumulate --use-unencrypted-wallet blocks minor acc://bvn-bvn0 10 5 countOnly >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieMinor "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API count-only on DN"
accumulate --use-unencrypted-wallet blocks minor acc://dn 10 5 countOnly >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieMinor "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API expand on BVN0"
accumulate --use-unencrypted-wallet blocks minor acc://bvn-bvn0 10 50 expand excludeempty >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieMinor "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API expand on DN"
accumulate --use-unencrypted-wallet blocks minor acc://dn 10 50 expand >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieMinor "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API expand on all networks"
accumulate --use-unencrypted-wallet blocks minor all-networks 10 50 expand >/tmp/minor.txt
FILESIZE=$(stat -c%s "/tmp/minor.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieMinor "minor block result set too small: ${FILESIZE}"
fi

success

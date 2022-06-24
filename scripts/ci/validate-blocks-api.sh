#!/bin/bash

# Stop immediately on error
set -e

SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
source ${SCRIPT_DIR}/validate-commons.sh

# die <message> - Print an error message and exit
function dieBlocksApi {
  echo -e '\033[1;31m'"$@"'\033[0m'
  echo the output was:
  cat /tmp/output.txt
  exit 1
}

section "Test minor block API omit-tx on BVN0"
accumulate --use-unencrypted-wallet blocks minor acc://bvn-bvn0 10 5 omit >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieBlocksApi "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API omit-tx on DN"
accumulate --use-unencrypted-wallet blocks minor acc://dn 10 5 omit >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieBlocksApi "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API count-only on BVN0"
accumulate --use-unencrypted-wallet blocks minor acc://bvn-bvn0 10 5 countOnly >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieBlocksApi "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API count-only on DN"
accumulate --use-unencrypted-wallet blocks minor acc://dn 10 5 countOnly >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieBlocksApi "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API expand on BVN0"
accumulate --use-unencrypted-wallet blocks minor acc://bvn-bvn0 10 50 expand excludeempty >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieBlocksApi "minor block result set too small: ${FILESIZE}"
fi

section "Test minor block API expand on DN"
accumulate --use-unencrypted-wallet blocks minor acc://dn 10 50 expand >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "500" ]; then
  dieBlocksApi "minor block result set too small: ${FILESIZE}"
fi

section "Test major block API on DN"
accumulate --use-unencrypted-wallet blocks major acc://dn 1 1 >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "300" ]; then
  dieBlocksApi "major block result set too small: ${FILESIZE}"
fi

section "Test major block API on BVN0"
accumulate --use-unencrypted-wallet blocks major acc://bvn-bvn0 1 1 >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "400" ]; then
  dieBlocksApi "major block result set too small: ${FILESIZE}"
fi

section "Test major block API on BVN1"
accumulate --use-unencrypted-wallet blocks major acc://bvn-bvn1 1 1 >/tmp/output.txt
FILESIZE=$(stat -c%s "/tmp/output.txt")
if [ "${FILESIZE}" -lt "400" ]; then
  dieBlocksApi "major block result set too small: ${FILESIZE}"
fi

success

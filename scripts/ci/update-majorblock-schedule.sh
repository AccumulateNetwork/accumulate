#!/bin/bash

# Stop immediately on error
set -e
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${SCRIPT_DIR}"/validate-commons.sh

# Format the path to priv_validator_key.json
function dnPrivKey {
  echo "$NODES_DIR/node-$1/priv_validator_key.json"
}

function signCount {
   echo "$(bc -l <<<"$ACCEPT_THRESHOLD")"
}


function daemon-run {
    if ! RESULT=`accumulated "$@" 2>&1`; then
        echo "$RESULT" >&2
        >&2 echo -e '\033[1;31m'"$@"'\033[0m'
        return 1
    fi
    echo "$RESULT"
}

# cli-tx <args...> - Execute a CLI command and extract the transaction hash from the result
function daemon-tx {
    RESULT=`daemon-run "$@"` || return 1
    echo "$RESULT"  | grep "Hash:" | cut -f2 -d" "
}

section "Setup"
if which go >/dev/null || ! which accumulate >/dev/null; then
  echo "Installing CLI & daemon"
  go install ./cmd/accumulate
  go install ./cmd/accumulated
  export PATH="${PATH}:$(go env GOPATH)/bin"
fi

[ -z "${MNEMONIC}" ] || accumulate key import mnemonic ${MNEMONIC}
echo

declare -g NUM_NODES=$(find ${NODES_DIR} -mindepth 1 -maxdepth 1 -type d | wc -l)
declare -g ACCEPT_THRESHOLD=$(accumulate page get -j dn.acme/operators/1 | jq -re .data.acceptThreshold)

section "Set major block time to 1 minute"
TXID=$(daemon-tx -w "${NODES_DIR}/node-1/dnn" set schedule "* * * * *")
echo RESULT: |$TXID|
wait-for-tx $TXID

# Sign the required number of times
echo Signature count $(signCount)
for ((sigNr = 1; sigNr < $(signCount); sigNr++)); do
  echo Signature $sigNr
  wait-for cli-tx-sig tx sign dn.acme/operators "$(dnPrivKey $sigNr)" $TXID
done
accumulate -j tx get $TXID | jq -re .status.pending 1>/dev/null && die "Transaction is pending"
accumulate -j tx get $TXID | jq -re .status.delivered 1>/dev/null || die "Transaction was not delivered"

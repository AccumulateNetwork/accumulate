#!/bin/bash

# Stop immediately on error
set -e

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${SCRIPT_DIR}"/validate-commons.sh
# Format the path to priv_validator_key.json
function dnPrivKey {
  echo "$NODES_DIR/priv_validator_key.json"
}

function signCount {
  echo "$(bc -l <<<"$ACCEPT_THRESHOLD")"
}

section "Setup validate-dual-mode"
if which go >/dev/null || ! which accumulate >/dev/null; then
  echo "Installing CLI & daemon"
  go install ./cmd/accumulate
  go install ./cmd/accumulated
  export PATH="${PATH}:$(go env GOPATH)/bin"
fi
[ -z "${MNEMONIC}" ] || accumulate key import mnemonic ${MNEMONIC}
echo

#spin up a dual node 
  accumulated init dual tcp://node-1:26656 --public=tcp://127.0.1.101 --listen=tcp://127.0.1.101 -w "$NODES_DIR/dual-test" --skip-version-check --no-website --skip-peer-health-check  || die "init dual mode failed"

  # Start the new validator and increment NUM_DMNS
  accumulated run-dual "$NODES_DIR/dual-test/dnn" "$NODES_DIR/dual-test/bvnn" &
  declare -g ACCPID=$!


if [ ! -z "${ACCPID}" ]; then
  section "Shutdown dual mode node validator"
  TXID=$(cli-tx validator remove dn "$(dnPrivKey 1)" "$hexPubKey")
  wait-for-tx $TXID

  # Sign the required number of times
  echo Signature count $(signCount)
  for ((sigNr = 2; sigNr <= $(signCount); sigNr++)); do
    echo Signature $sigNr
    wait-for cli-tx-sig tx sign dn.acme/operators "$(dnPrivKey $sigNr)" $TXID
  done
  accumulate -j tx get $TXID | jq -re .status.pending 1>/dev/null && die "Transaction is pending"
  accumulate -j tx get $TXID | jq -re .status.delivered 1>/dev/null || die "Transaction was not delivered"

  kill -9 $ACCPID || true
fi

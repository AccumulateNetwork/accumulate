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
accumulated init dual tcp://node-1:26756 --public=tcp://127.0.1.101 --listen=tcp://127.0.1.101 -w "$NODES_DIR/dual-test" --skip-version-check --no-website && success || die "init dual mode failed"

# Start the new validator and increment NUM_DMNS
accumulated run-dual "$NODES_DIR/dual-test/dnn" "$NODES_DIR/dual-test/bvnn" &
declare -g ACCPID=$!


section "Generate a Lite Token Account"
accumulate account list 2>&1 | grep -q ACME || accumulate account generate
LITE_ACME=$(accumulate account list -j | jq -re .liteAccounts[0].liteAccount)
LITE_ID=$(cut -d/ -f-3 <<< "$LITE_ACME")
TXS=()
for i in {1..1}
do
	TXS=(${TXS[@]} $(cli-tx -s http://127.0.1.101:26660/v2 faucet ${LITE_ACME}))
done
for tx in "${TXS[@]}"
do
	echo $tx
	wait-for-tx $tx
done

//make sure no errors occurred
accumulate account get ${LITE_ACME} 1> /dev/null && success || die "Cannot find ${LITE_ACME}"

if [ ! -z "${ACCPID}" ]; then
  kill -9 $ACCPID || true
fi

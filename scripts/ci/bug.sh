#!/bin/bash

# Stop immediately on error
set -e

SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
source ${SCRIPT_DIR}/validate-commons.sh
section "Setup"
if which go > /dev/null || ! which accumulate > /dev/null ; then
    echo "Installing CLI"
    go install ./cmd/accumulate
    export PATH="${PATH}:$(go env GOPATH)/bin"
fi
init-wallet
echo

section "Generate a Lite Token Account"
accumulate account list 2>&1 | grep -q ACME || accumulate account generate
LITE_ACME=$(accumulate account list -j | jq -re .liteAccounts[0].liteAccount)
LITE_ID=$(cut -d/ -f-3 <<< "$LITE_ACME")
TXS=()
for i in {1..1}
do
	TXS=(${TXS[@]} $(cli-tx faucet ${LITE_ACME}))
done
for tx in "${TXS[@]}"
do
	echo $tx
	wait-for-tx $tx
done

accumulate account get ${LITE_ACME} 1> /dev/null && success || die "Cannot find ${LITE_ACME}"

section "Add credits to lite account"
TXID=$(cli-tx credits ${LITE_ACME} ${LITE_ID} 1000000)
wait-for-tx $TXID
BALANCE=$(accumulate -j account get ${LITE_ID} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 2700 ] || die "${LITE_ID} should have at least 2700 credits but only has ${BALANCE}"
success

section "Generate keys"
ensure-key test-1-0
echo

section "Create an ADI"
wait-for cli-tx adi create ${LITE_ID} test.acme test-1-0 test.acme/book
accumulate adi get test.acme 1> /dev/null && success || die "Cannot find keytest"

section "Add credits to the ADI's key page 1"
wait-for cli-tx credits ${LITE_ACME} test.acme/book/1 60000
BALANCE=$(accumulate -j page get test.acme/book/1 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 6000000 ] && success || die "test.acme/book/1 should have 6000000 credits but has ${BALANCE}"

section "Create additional Key Pages"
wait-for cli-tx page create test.acme/book test-1-0 test-1-0
wait-for cli-tx page create test.acme/book test-1-0 test-1-0
accumulate page get test.acme/book/2 1> /dev/null || die "Cannot find page test.acme/book/2"
accumulate page get test.acme/book/3 1> /dev/null || die "Cannot find page test.acme/book/3"
success

section "Add credits to the ADI's key page 2"
wait-for cli-tx credits ${LITE_ACME} test.acme/book/2 1000
BALANCE=$(accumulate -j page get test.acme/book/2 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 1000 ] && success || die "test.acme/book/2 should have 1000 credits but has ${BALANCE}"

section "Execute transaction using page2"
BALANCE1=$(accumulate -j page get test.acme/book/2 | jq -r .data.creditBalance)
txHash=$(cli-tx tx execute test.acme/book/2 test-1-0@test.acme/book/2 '{"type": "updateKeyPage", "operation": [{ "type": "add", "entry":{"delegate": "acc://test.acme/book2"}}]}')
wait-for-tx $txHash
echo $txHash
BALANCE2=$(accumulate -j page get test.acme/book/2 | jq -r .data.creditBalance)
echo $BALANCE1 $BALANCE2
[ "$BALANCE1" -eq "$BALANCE2" ] && die "credits deducted from wrong keypage" || success

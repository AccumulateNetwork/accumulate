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
ensure-key test-2-0
ensure-key test-2-1
ensure-key test-2-2
ensure-key test-2-3-orig
ensure-key test-2-3-new
ensure-key test-3-0
ensure-key test-3-1
ensure-key test-mgr
echo

section "Create lite data account and write the data"
JSON=$(accumulate -j account create data --lite test.acme test-1-0 "Factom PRO" "Tutorial")
wait-for-tx $(jq -r .transactionHash <<< "$JSON")
ACCOUNT_ID=$(jq -r .accountUrl <<< "$JSON")zz
[ "$ACCOUNT_ID" == "acc://b36c1c4073305a41edc6353a094329c24ffa54c0a47fb56227a04477bcb78923" ] || die "${ACCOUNT_ID} does not match expected value"
accumulate data get $ACCOUNT_ID 0 1 1> /dev/null || die "lite data entry not found"
wait-for cli-tx data write-to test.acme test-1-0 $ACCOUNT_ID "data test"
accumulate data get $ACCOUNT_ID 0 2 1> /dev/null || die "lite data error"
accumulate -j get "${ACCOUNT_ID}#txn/0" | jq -re .status.result.entryHash &> /dev/null || die "Entry hash is missing from transaction results"
accumulate -j get "${ACCOUNT_ID}#txn/0" | jq -re .status.result.accountID &> /dev/null || die "Account ID is missing from transaction results"
success

section "Create lite data account with first entry"
ACCOUNT_ID=$(accumulate -j account create data --lite test.acme test-1-0 "First Data Entry" "Check" --lite-data "first entry" | jq -r .accountUrl)
[ "$ACCOUNT_ID" == "acc://4df014cc532c140066add495313e0ffaecba1eba2a4fb95e30d37fc3f87e9ab8" ] || die "${ACCOUNT_ID} does not match expected value"
accumulate -j data get $ACCOUNT_ID 0 1 1> /dev/null || die "lite data entry not found"
wait-for cli-tx data write-to test.acme test-1-0 $ACCOUNT_ID "data test"
accumulate data get $ACCOUNT_ID 0 2 1> /dev/null || die "lite data error"
accumulate -j get "${ACCOUNT_ID}#txn/0" | jq -re .status.result.entryHash &> /dev/null || die "Entry hash is missing from transaction results"
accumulate -j get "${ACCOUNT_ID}#txn/0" | jq -re .status.result.accountID &> /dev/null || die "Account ID is missing from transaction results"
success

section "Create ADI Data Account"
wait-for cli-tx account create data test.acme test-1-0 test.acme/data
accumulate account get test.acme/data 1> /dev/null || die "Cannot find test.acme/data"
success

section "Write data to ADI Data Account"
JSON=$(accumulate -j data write --scratch test.acme/data test-1-0 foo bar)
TXID=$(echo $JSON | jq -re .transactionHash)
echo $JSON | jq -C --indent 0
wait-for-tx $TXID
echo $JSON | jq -re .result.result.entryHash 1> /dev/null || die "Deliver response does not include the entry hash"
accumulate -j tx get $TXID | jq -re .status.result.entryHash 1> /dev/null || die "Transaction query response does not include the entry hash"
success

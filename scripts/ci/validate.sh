#!/bin/bash

set -e # Stop immediately on error

function section {
    echo -e '\033[1m'"$1"'\033[0m'
}

function ensure-key {
    if ! accumulate key list | grep "$1"; then
        accumulate key generate "$1"
    fi
}

function wait-for {
    TXID=`"$@"` || return 1
    echo -e '\033[2mWaiting for '"$TXID"'\033[0m'
    accumulate tx get -j --wait 10s --wait-synth 10s $TXID | jq --indent 0
}

function cli-tx {
    JSON=`accumulate -j "$@"` || return 1
    echo "$JSON" | jq -r .txid
}

function api-v2 {
    curl -s -X POST --data "${1}" -H 'content-type:application/json;' "${ACC_API}/../v2"
}

function api-tx {
    JSON=`api-v2 "$@"` || return 1
    echo "$JSON" | jq -r .result.txid
}

function die {
    echo -e '\033[1;31m'"$@"'\033[0m'
    exit 1
}

function success {
    echo -e '\033[1;32m'Success'\033[0m'
    echo
}

section "Setup"
if ! which accumulate > /dev/null ; then
    go install ./cmd/accumulate
    export PATH="${PATH}:$(go env GOPATH)/bin"
fi
[ -z "${MNEMONIC}" ] || accumulate key import mnemonic ${MNEMONIC}
echo

section "Generate a Lite Token Account"
accumulate account list | grep -q ACME || accumulate account generate
LITE=$(accumulate account list | grep ACME | head -1)
wait-for cli-tx faucet ${LITE}
accumulate account get ${LITE} &> /dev/null && success || die "Cannot find ${LITE}"

section "Add credits to lite account"
wait-for cli-tx credits ${LITE} ${LITE} 100
BALANCE=$(accumulate -j account get ${LITE} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 100 ] && success || die "${LITE} should have at least 100 credits but only has ${BALANCE}"

section "Generate keys"
ensure-key keytest-0-0
ensure-key keytest-1-0
ensure-key keytest-1-1
ensure-key keytest-2-0
ensure-key keytest-2-1
echo

section "Create an ADI"
wait-for cli-tx adi create ${LITE} keytest keytest-0-0 book page0
accumulate adi get keytest &> /dev/null && success || die "Cannot find keytest"

section "Create additional Key Pages"
wait-for cli-tx page create keytest/book keytest-0-0 keytest/page1 keytest-1-0
wait-for cli-tx page create keytest/book keytest-0-0 keytest/page2 keytest-2-0
accumulate page get keytest/page1 &> /dev/null || die "Cannot find keytest/page1"
accumulate page get keytest/page2 &> /dev/null || die "Cannot find keytest/page2"
success

section "Add a key to page 1 using a key from page 1"
wait-for cli-tx page key add keytest/page1 keytest-1-0 1 keytest-1-1
success

section "Add a key to page 2 using a key from page 1"
wait-for cli-tx page key add keytest/page2 keytest-1-0 1 keytest-2-1
success

section "Create an ADI Token Account"
wait-for cli-tx account create token keytest keytest-0-0 0 keytest/tokens ACME keytest/book
accumulate account get keytest/tokens &> /dev/null && success || die "Cannot find keytest/tokens"

section "Send tokens from the lite token account to the ADI token account"
wait-for cli-tx tx create ${LITE} keytest/tokens 5
BALANCE=$(accumulate -j account get keytest/tokens | jq -r .data.balance)
[ "$BALANCE" -eq 500000000 ] && success || die "${LITE} should have 5 tokens but has $(expr ${BALANCE} / 100000000)"

section "Add credits to the ADI's key page 0"
wait-for cli-tx credits keytest/tokens keytest-0-0 0 keytest/page0 125
BALANCE=$(accumulate -j page get keytest/page0 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 125 ] && success || die "keytest/page0 should have 125 credits but has ${BALANCE}"

section "Bug AC-551"
api-v2 '{"jsonrpc": "2.0", "id": 4, "method": "metrics", "params": {"metric": "tps", "duration": "1h"}}' | jq -e .result.data.value &> /dev/null
success

section "API v2 faucet (AC-570)"
BEFORE=$(accumulate -j account get ${LITE} | jq -r .data.balance)
wait-for api-tx '{"jsonrpc": "2.0", "id": 4, "method": "faucet", "params": {"url": "'${LITE}'"}}'
AFTER=$(accumulate -j account get ${LITE} | jq -r .data.balance)
DIFF=$(expr $AFTER - $BEFORE)
[ $DIFF -eq 1000000000 ] && success || die "Faucet did not work, want +1000000000, got ${DIFF}"

section "Parse acme faucet TXNs (API v2, AC-603)"
api-v2 '{ "jsonrpc": "2.0", "id": 0, "method": "query-tx-history", "params": { "url": "7117c50f04f1254d56b704dc05298912deeb25dbc1d26ef6/ACME", "count": 10 } }' | jq -r '.result.items | map(.type)[]' | grep -q acmeFaucet
success

section "Include Merkle state (API, AC-604)"
accumulate -j adi get keytest | jq -e .mainChain.roots &> /dev/null || die "Failed: response does not include main chain roots"
accumulate -j adi get keytest | jq -e .mainChain.height &> /dev/null || die "Failed: response does not include main chain height"
api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query", "params": {"url": "keytest"}}' | jq -e .result.mainChain.roots &> /dev/null
api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query", "params": {"url": "keytest"}}' | jq -e .result.mainChain.height &> /dev/null
success

section "Query with txid and chainId (API v2, AC-602)"
# TODO Verify query-chain
TXID=$(accumulate -j tx history keytest 0 1 | jq -re '.items[0].txid')
GOT=$(api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query-tx", "params": {"txid": "'${TXID}'"}}' | jq -re .result.txid)
[ "${TXID}" = "${GOT}" ] || die "Failed to find TX ${TXID}"
success
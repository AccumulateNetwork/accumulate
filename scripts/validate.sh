#!/bin/bash

set -e # Stop immediately on error

function section {
    echo -e '\033[1m'"$1"'\033[0m'
}

function ensure-key {
    if ! cli key list 2>&1 | grep "$1"; then
        cli key generate "$1"
    fi
}

function api-v2 {
    curl -s -X POST --data "${1}" -H 'content-type:application/json;' "${ACC_API}/../v2"
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
go install ./cmd/cli
export PATH="${PATH}:$(go env GOPATH)/bin"
[ -z "${MNEMONIC}" ] || cli key import mnemonic ${MNEMONIC}
echo

section "Generate a Lite Token Account"
cli account list 2>&1 | grep -q ACME || cli account generate
LITE=$(cli account list 2>&1 | grep ACME | head -1)
cli faucet ${LITE}
sleep 3
cli account get ${LITE} &> /dev/null && success || die "Cannot find ${LITE}"

section "Add credits to lite account"
cli credits ${LITE} ${LITE} 100
sleep 3
BALANCE=$(cli -j account get ${LITE} 2>&1 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 100 ] && success || die "${LITE} should have at least 100 credits but only has ${BALANCE}"

section "Generate keys"
ensure-key keytest-0-0
ensure-key keytest-1-0
ensure-key keytest-1-1
ensure-key keytest-2-0
ensure-key keytest-2-1
echo

section "Create an ADI"
cli adi create ${LITE} keytest keytest-0-0 book page0
sleep 3
cli adi get keytest &> /dev/null && success || die "Cannot find keytest"

section "Create additional Key Pages"
cli page create keytest/book keytest-0-0 keytest/page1 keytest-1-0
cli page create keytest/book keytest-0-0 keytest/page2 keytest-2-0
sleep 3
cli page get keytest/page1 &> /dev/null || die "Cannot find keytest/page1"
cli page get keytest/page2 &> /dev/null || die "Cannot find keytest/page2"
success

# section "Add a key to page 1 using a key from page 1"
# cli page key add keytest/page1 keytest-1-0 1 1 keytest-1-1

# section "Add a key to page 2 using a key from page 1"
# cli page key add keytest/page2 keytest-1-0 1 1 keytest-2-1

section "Create an ADI Token Account"
cli account create keytest keytest-0-0 0 1 keytest/tokens ACME keytest/book
sleep 3
cli account get keytest/tokens &> /dev/null && success || die "Cannot find keytest/tokens"

section "Send tokens from the lite token account to the ADI token account"
cli tx create ${LITE} keytest/tokens 5
sleep 3
BALANCE=$(cli -j account get keytest/tokens 2>&1 | jq -r .data.balance)
[ "$BALANCE" -eq 500000000 ] && success || die "${LITE} should have 5 tokens but has $(expr ${BALANCE} / 100000000)"

section "Add credits to the ADI's key page 0"
cli credits keytest/tokens keytest-0-0 0 1 keytest/page0 125
sleep 3
BALANCE=$(cli -j page get keytest/page0 2>&1 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 125 ] && success || die "keytest/page0 should have 125 credits but has ${BALANCE}"

section "Bug AC-517"
WANT=0000000000000000000000000000000000000000000000000000000000000000
GOT=$(cli -j book get keytest/book 2>&1 | jq -r .data.sigSpecId)
[ "${WANT}" = "${GOT}" ] && success || die "Failed"

section "Bug AC-551"
api-v2 '{"jsonrpc": "2.0", "id": 4, "method": "metrics", "params": {"metric": "tps", "duration": "1h"}}' | jq -e .result.data.value &> /dev/null
success

section "API v2 faucet (AC-570)"
BEFORE=$(cli -j account get ${LITE} 2>&1 | jq -r .data.balance)
api-v2 '{"jsonrpc": "2.0", "id": 4, "method": "faucet", "params": {"url": "'${LITE}'"}}'
sleep 3
AFTER=$(cli -j account get ${LITE} 2>&1 | jq -r .data.balance)
DIFF=$(expr $AFTER - $BEFORE)
[ $DIFF -eq 1000000000 ] && success || die "Faucet did not work, want +1000000000, got ${DIFF}"
echo

section "Parse acme faucet TXNs (API v2, AC-603)"
api-v2 '{ "jsonrpc": "2.0", "id": 0, "method": "query-tx-history", "params": { "url": "7117c50f04f1254d56b704dc05298912deeb25dbc1d26ef6/ACME", "count": 10 } }' | jq -r '.result.items | map(.type)[]' | grep -q acmeFaucet
success

section "Include Merkle state (API, AC-604)"
cli -j adi get keytest 2>&1 | jq -e .merkleState.roots &> /dev/null
cli -j adi get keytest 2>&1 | jq -e .merkleState.count &> /dev/null
api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query", "params": {"url": "keytest"}}' | jq -e .result.merkleState.roots &> /dev/null
api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query", "params": {"url": "keytest"}}' | jq -e .result.merkleState.count &> /dev/null
success

section "Query with txid and chainId (API v2, AC-602)"
# TODO Verify query-chain
TXID=$(cli -j tx history keytest 0 1 2>&1 | jq -re '.data[0].txid')
GOT=$(api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query-tx", "params": {"txid": "'${TXID}'"}}' | jq -re .result.txid)
[ "${TXID}" = "${GOT}" ] || die "Failed to find TX ${TXID}"
success
#!/bin/bash

# Stop immediately on error
set -e

# section <name> - Print a section header
function section {
    echo -e '\033[1m'"$1"'\033[0m'
}

# ensure-key <name> - Generate the key if it does not exist
function ensure-key {
    if ! accumulate key list | grep "$1"; then
        accumulate key generate "$1"
    fi
}

# wait-for <cmd...> - Execute a transaction and wait for it to complete
function wait-for {
    if [ "$1" == "--no-check" ]; then
        local NO_CHECK=$1
        shift
    fi
    local TXID
    TXID=`"$@"` || return 1
    wait-for-tx $NO_CHECK "$TXID" || return 1
}

# wait-for-tx [--no-check] <tx id> - Wait for a transaction and any synthetic transactions to complete
function wait-for-tx {
    if [ "$1" == "--no-check" ]; then
        local NO_CHECK=$1
        shift
    fi

    local TXID=$1
    echo -e '\033[2mWaiting for '"$TXID"'\033[0m'
    local RESP=$(accumulate tx get -j --wait 10s $TXID)
    echo $RESP | jq -C --indent 0

    if [ -z "$NO_CHECK" ]; then
        CODE=$(echo $RESP | jq -re '.status.code // 0') || return 1
        [ "$CODE" -ne 0 ] && die "$TXID failed:" $(echo $RESP | jq -C --indent 0 .status)
    fi

    for TXID in $(echo $RESP | jq -re '(.syntheticTxids // [])[]'); do
        wait-for-tx $NO_CHECK "$TXID"
    done
}

# cli-tx <args...> - Execute a CLI command and extract the TXID from the result
function cli-tx {
    JSON=`accumulate -j "$@"` || return 1
    echo "$JSON" | jq -re .txid
}

# api-v2 <payload> - Send a JSON-RPC message to the API
function api-v2 {
    curl -s -X POST --data "${1}" -H 'content-type:application/json;' "${ACC_API}/../v2"
}

# api-tx <payload> - Send a JSON-RPC message to the API and extract the TXID from the result
function api-tx {
    JSON=`api-v2 "$@"` || return 1
    echo "$JSON" | jq -r .result.txid
}

# die <message> - Print an error message and exit
function die {
    echo -e '\033[1;31m'"$@"'\033[0m'
    exit 1
}

# success - Print 'success' in bold green text
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
TX0=$(cli-tx faucet ${LITE})
TX1=$(cli-tx faucet ${LITE})
TX2=$(cli-tx faucet ${LITE})
TX3=$(cli-tx faucet ${LITE})
TX4=$(cli-tx faucet ${LITE})
TX5=$(cli-tx faucet ${LITE})
TX6=$(cli-tx faucet ${LITE})
TX7=$(cli-tx faucet ${LITE})
TX8=$(cli-tx faucet ${LITE})
TX9=$(cli-tx faucet ${LITE})
wait-for-tx $TX0
wait-for-tx $TX1
wait-for-tx $TX2
wait-for-tx $TX3
wait-for-tx $TX4
wait-for-tx $TX5
wait-for-tx $TX6
wait-for-tx $TX7
wait-for-tx $TX8
wait-for-tx $TX9
accumulate account get ${LITE} &> /dev/null && success || die "Cannot find ${LITE}"

section "Add credits to lite account"
wait-for cli-tx credits ${LITE} ${LITE} 1100
BALANCE=$(accumulate -j account get ${LITE} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 1100 ] && success || die "${LITE} should have at least 1100 credits but only has ${BALANCE}"

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

section "Verify fee charge"
BALANCE=$(accumulate -j account get ${LITE} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 100 ] && success || die "${LITE} should have at least 100 credits but only has ${BALANCE}"

section "Recreating an ADI fails and the synthetic transaction is recorded"
TXID=`cli-tx adi create ${LITE} keytest keytest-1-0 book page1` || return 1
wait-for-tx --no-check $TXID
SYNTH=`accumulate tx get -j ${TXID} | jq -re '.syntheticTxids[0]'`
STATUS=`accumulate tx get -j ${SYNTH} | jq --indent 0 .status`
[ $(echo $STATUS | jq -re .delivered) = "true" ] || die "Synthetic transaction was not delivered"
[ $(echo $STATUS | jq -re '.code // 0') -ne 0 ] || die "Synthetic transaction did not fail"
echo $STATUS | jq -re .message &> /dev/null || die "Synthetic transaction does not have a message"
success

section "Add credits to the ADI's key page 0"
wait-for cli-tx credits ${LITE} keytest/page0 5500
BALANCE=$(accumulate -j page get keytest/page0 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 5500 ] && success || die "keytest/page0 should have 5500 credits but has ${BALANCE}"

section "Create additional Key Pages"
wait-for cli-tx page create keytest/book keytest-0-0 keytest/page1 keytest-1-0
wait-for cli-tx page create keytest/book keytest-0-0 keytest/page2 keytest-2-0
accumulate page get keytest/page1 &> /dev/null || die "Cannot find keytest/page1"
accumulate page get keytest/page2 &> /dev/null || die "Cannot find keytest/page2"
success

section "Add credits to the ADI's key page 1"
wait-for cli-tx credits ${LITE} keytest/page1 100
BALANCE=$(accumulate -j page get keytest/page1 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 100 ] && success || die "keytest/page1 should have 100 credits but has ${BALANCE}"

section "Add a key to page 1 using a key from page 1"
wait-for cli-tx page key add keytest/page1 keytest-1-0 1 keytest-1-1
success

section "Add a key to page 2 using a key from page 1"
wait-for cli-tx page key add keytest/page2 keytest-1-0 1 keytest-2-1
success

section "Set threshold to 2 of 2"
wait-for cli-tx tx execute keytest/page1 keytest-1-0 '{"type": "updateKeyPage", "operation": "setThreshold", "threshold": 2}'
THRESHOLD=$(accumulate -j get keytest/page1 | jq -re .data.threshold)
[ "$THRESHOLD" -eq 2 ] && success || die "Bad keytest/page1 threshold: want 2, got ${THRESHOLD}"

section "Create an ADI Token Account"
wait-for cli-tx account create token keytest keytest-0-0 0 keytest/tokens ACME keytest/book
accumulate account get keytest/tokens &> /dev/null && success || die "Cannot find keytest/tokens"

section "Send tokens from the lite token account to the ADI token account"
wait-for cli-tx tx create ${LITE} keytest/tokens 5
BALANCE=$(accumulate -j account get keytest/tokens | jq -r .data.balance)
[ "$BALANCE" -eq 500000000 ] && success || die "${LITE} should have 5 tokens but has $(expr ${BALANCE} / 100000000)"

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

section "Create a token issuer"
wait-for cli-tx tx execute keytest keytest-0-0 '{"type": "createToken", "url": "keytest/token-issuer", "symbol": "TOK", "precision": 10}'
accumulate get keytest/token-issuer &> /dev/null || die "Cannot find keytest/token-issuer"
success

section "Issue tokens"
LITE_TOK=$(echo $LITE | cut -d/ -f-3)/keytest/token-issuer
wait-for cli-tx tx execute keytest/token-issuer keytest-0-0 '{"type": "issueTokens", "recipient": "'${LITE_TOK}'", "amount": "123"}'
BALANCE=$(accumulate -j account get ${LITE_TOK} | jq -r .data.balance)
[ "$BALANCE" -eq 123 ] && success || die "${LITE_TOK} should have 123 keytest tokens but has ${BALANCE}"

section "Add credits to lite account (TOK)"
wait-for cli-tx credits ${LITE} ${LITE_TOK} 100
BALANCE=$(accumulate -j account get ${LITE_TOK} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 100 ] && success || die "${LITE_TOK} should have at least 100 credits but only has ${BALANCE}"

section "Burn tokens"
wait-for cli-tx tx execute ${LITE_TOK} '{"type": "burnTokens", "amount": "100"}'
BALANCE=$(accumulate -j account get ${LITE_TOK} | jq -r .data.balance)
[ "$BALANCE" -eq 23 ] && success || die "${LITE_TOK} should have 23 keytest tokens but has ${BALANCE}"

section "Create lite data account and write the data"
ACCOUNT_ID=$(accumulate -j account create data lite keytest keytest-0-0 "Factom PRO" "Tutorial" | jq -r .accountUrl)
[ "$ACCOUNT_ID" == "acc://b36c1c4073305a41edc6353a094329c24ffa54c029a521aa" ] || die "${ACCOUNT_ID} does not match expected value"
accumulate data get $ACCOUNT_ID 0 1 &> /dev/null || die "lite data entry not found"
wait-for cli-tx data write-to keytest keytest-0-0 $ACCOUNT_ID "data test"
accumulate data get $ACCOUNT_ID 0 2 &> /dev/null || die "lite data error"
success

section "Create ADI Data Account"
wait-for cli-tx account create data keytest keytest-0-0 keytest/data
accumulate account get keytest/data &> /dev/null && success || die "Cannot find keytest/data"

section "Write data to ADI Data Account"
JSON=$(accumulate -j data write keytest/data keytest-0-0 foo bar)
TXID=$(echo $JSON | jq -re .txid)
echo $JSON | jq -C --indent 0
wait-for-tx $TXID
echo $JSON | jq -re .result.entryHash &> /dev/null || die "Deliver response does not include the entry hash"
accumulate -j tx get $TXID | jq -re .status.result.entryHash &> /dev/null || die "Transaction query response does not include the entry hash"
success
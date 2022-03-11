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
        wait-for-tx $NO_CHECK "$TXID" || return 1
    done
}

# cli-tx <args...> - Execute a CLI command and extract the transaction hash from the result
function cli-tx {
    JSON=`accumulate -j "$@"` || return 1
    echo "$JSON" | jq -re .transactionHash
}

# cli-tx-env <args...> - Execute a CLI command and extract the envelope hash from the result
function cli-tx-env {
    JSON=`accumulate -j "$@"` || return 1
    echo "$JSON" | jq -re .envelopeHash
}

# api-v2 <payload> - Send a JSON-RPC message to the API
function api-v2 {
    curl -s -X POST --data "${1}" -H 'content-type:application/json;' "${ACC_API}/../v2"
}

# api-tx <payload> - Send a JSON-RPC message to the API and extract the TXID from the result
function api-tx {
    JSON=`api-v2 "$@"` || return 1
    echo "$JSON" | jq -r .result.transactionHash
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

NODE_PRIV_VAL="${NODE_ROOT:-~/.accumulate/dn/Node0}/config/priv_validator_key.json"

section "Update oracle price to 1 dollar. Oracle price has precision of 4 decimals"
if [ -f "$NODE_PRIV_VAL" ]; then
    wait-for cli-tx data write dn/oracle "$NODE_PRIV_VAL" '{"price":501}'
    RESULT=$(accumulate -j data get dn/oracle)
    RESULT=$(echo $RESULT | jq -re .data.entry.data | xxd -r -p | jq -re .price)
    [ "$RESULT" == "501" ] && success || die "cannot update price oracle"
else
    echo -e '\033[1;31mCannot update oracle: private validator key not found\033[0m'
fi

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
TXS=()
for i in {1..20}
do
	TXS=(${TXS[@]} $(cli-tx faucet ${LITE}))
done
for tx in "${TXS[@]}"
do
	echo $tx
	wait-for-tx $tx
done

accumulate account get ${LITE} 1> /dev/null && success || die "Cannot find ${LITE}"

section "Add credits to lite account"
TXID=$(cli-tx credits ${LITE} ${LITE} 2200)
wait-for-tx $TXID
BALANCE=$(accumulate -j account get ${LITE} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 2200 ] || die "${LITE} should have at least 2200 credits but only has ${BALANCE}"
TXID=$(accumulate -j tx get ${TXID} | jq -re .syntheticTxids[1]) # this depends on the burn coming last
TYPE=$(accumulate -j tx get ${TXID} | jq -re .type)
[ "$TYPE" == "syntheticBurnTokens" ] || die "Expected a syntheticBurnTokens, got ${TYPE}"
success

section "Generate keys"
ensure-key keytest-1-0
ensure-key keytest-2-0
ensure-key keytest-2-1
ensure-key keytest-2-2
ensure-key keytest-2-0
ensure-key keytest-3-1
echo

section "Create an ADI"
wait-for cli-tx adi create ${LITE} keytest keytest-1-0 keytest/book
accumulate adi get keytest 1> /dev/null && success || die "Cannot find keytest"

section "Verify fee charge"
BALANCE=$(accumulate -j account get ${LITE} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 100 ] && success || die "${LITE} should have at least 100 credits but only has ${BALANCE}"

section "Recreating an ADI fails and the synthetic transaction is recorded"
TXID=`cli-tx adi create ${LITE} keytest keytest-2-0 keytest/book` || return 1
wait-for-tx --no-check $TXID
SYNTH=`accumulate tx get -j ${TXID} | jq -re '.syntheticTxids[0]'`
STATUS=`accumulate tx get -j ${SYNTH} | jq --indent 0 .status`
[ $(echo $STATUS | jq -re .delivered) = "true" ] || die "Synthetic transaction was not delivered"
[ $(echo $STATUS | jq -re '.code // 0') -ne 0 ] || die "Synthetic transaction did not fail"
echo $STATUS | jq -re .message 1> /dev/null || die "Synthetic transaction does not have a message"
success

section "Add credits to the ADI's key page 1"
wait-for cli-tx credits ${LITE} keytest/book/1 60000
BALANCE=$(accumulate -j page get keytest/book/1 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 60000 ] && success || die "keytest/book/1 should have 60000 credits but has ${BALANCE}"

section "Create additional Key Pages"
wait-for cli-tx page create keytest/book keytest-1-0 keytest-2-0
wait-for cli-tx page create keytest/book keytest-1-0 keytest-2-0
accumulate page get keytest/book/2 1> /dev/null || die "Cannot find page keytest/book/2"
accumulate page get keytest/book/3 1> /dev/null || die "Cannot find page keytest/book/3"
success

section "Add credits to the ADI's key page 2"
wait-for cli-tx credits ${LITE} keytest/book/2 100
BALANCE=$(accumulate -j page get keytest/book/2 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 100 ] && success || die "keytest/book/2 should have 100 credits but has ${BALANCE}"

section "Add a key to page 2 using a key from page 3"
wait-for cli-tx page key add keytest/book/2 keytest-2-0 1 keytest-2-1
wait-for cli-tx page key add keytest/book/2 keytest-2-0 1 keytest-2-2
success

section "Add a key to page 2 using a key from page 1"
wait-for cli-tx page key add keytest/book/3 keytest-2-0 1 keytest-3-1
success

section "Set threshold to 2 of 2"
wait-for cli-tx tx execute keytest/book/2 keytest-2-0 '{"type": "updateKeyPage", "operation": "setThreshold", "threshold": 2}'
THRESHOLD=$(accumulate -j get keytest/book/2 | jq -re .data.threshold)
[ "$THRESHOLD" -eq 2 ] && success || die "Bad keytest/book/2 threshold: want 2, got ${THRESHOLD}"

section "Create an ADI Token Account"
wait-for cli-tx account create token --scratch keytest keytest-1-0 0 keytest/tokens ACME keytest/book
accumulate account get keytest/tokens 1> /dev/null || die "Cannot find keytest/tokens"
accumulate -j account get keytest/tokens | jq -re .data.scratch 1> /dev/null || die "keytest/tokens is not a scratch account"
success

section "Send tokens from the lite token account to the ADI token account"
wait-for cli-tx tx create ${LITE} keytest/tokens 5
BALANCE=$(accumulate -j account get keytest/tokens | jq -r .data.balance)
[ "$BALANCE" -eq 500000000 ] && success || die "${LITE} should have 5 tokens but has $(expr ${BALANCE} / 100000000)"

section "Send tokens from the ADI token account to the lite token account using the multisig page"
TXID=$(cli-tx tx create keytest/tokens keytest-2-0 ${LITE} 1)
wait-for-tx $TXID
accumulate -j tx get $TXID | jq -re .status.pending 1> /dev/null || die "Transaction is not pending"
accumulate -j tx get $TXID | jq -re .status.delivered 1> /dev/null && die "Transaction was delivered"
success

section "Signing the transaction with the same key does not deliver it"
wait-for cli-tx-env tx sign keytest/tokens keytest-2-0 $TXID
accumulate -j tx get $TXID | jq -re .status.pending 1> /dev/null || die "Transaction is not pending"
accumulate -j tx get $TXID | jq -re .status.delivered 1> /dev/null && die "Transaction was delivered"
wait-for-tx $TXID
success

section "Query pending by URL"
accumulate -j get keytest/tokens#pending | jq -re .items[0] &> /dev/null && success || die "Failed to retrieve pending transactions"


section "Query pending chain at height 0 by URL"
TXID=$(accumulate -j get keytest/tokens#pending/0 | jq -re .transactionHash) && success || die "Failed to query pending chain by height"

section "Query pending chain with hash by URL"
RESULT=$(accumulate -j get keytest/tokens#pending/${TXID} | jq -re .transactionHash) || die "Failed to query pending chain by hash"
[ "$RESULT" == "$TXID" ] && success || die "Querying by height and by hash gives different results"

section "Query pending chain range by URL"
RESULT=$(accumulate -j get keytest/tokens#pending/0:10 | jq -re .total)
[ "$RESULT" -ge 1 ] && success || die "No entries found"

section "Sign the pending transaction using the other key"
TXID=$(accumulate -j get keytest/tokens#pending | jq -re .items[0])
wait-for cli-tx-env tx sign keytest/tokens keytest-2-1 $TXID
accumulate -j tx get $TXID | jq -re .status.pending 1> /dev/null && die "Transaction is pending"
accumulate -j tx get $TXID | jq -re .status.delivered 1> /dev/null || die "Transaction was not delivered"
wait-for-tx $TXID
success

section "Signing the transaction after it has been delivered fails"
cli-tx-env tx sign keytest/tokens keytest-2-2 $TXID && die "Signed the transaction after it was delivered" || success

# section "Bug AC-551"
# api-v2 '{"jsonrpc": "2.0", "id": 4, "method": "metrics", "params": {"metric": "tps", "duration": "1h"}}' | jq -e .result.data.value 1> /dev/null
# success

section "API v2 faucet (AC-570)"
BEFORE=$(accumulate -j account get ${LITE} | jq -r .data.balance)
wait-for api-tx '{"jsonrpc": "2.0", "id": 4, "method": "faucet", "params": {"url": "'${LITE}'"}}'
AFTER=$(accumulate -j account get ${LITE} | jq -r .data.balance)
DIFF=$(expr $AFTER - $BEFORE)
[ $DIFF -eq 200000000000 ] && success || die "Faucet did not work, want +200000000000, got ${DIFF}"

section "Parse acme faucet TXNs (API v2, AC-603)"
api-v2 '{ "jsonrpc": "2.0", "id": 0, "method": "query-tx-history", "params": { "url": "7117c50f04f1254d56b704dc05298912deeb25dbc1d26ef6/ACME", "count": 10 } }' | jq -r '.result.items | map(.type)[]' | grep -q acmeFaucet
success

section "Include Merkle state (API, AC-604)"
accumulate -j adi get keytest | jq -e .mainChain.roots 1> /dev/null || die "Failed: response does not include main chain roots"
accumulate -j adi get keytest | jq -e .mainChain.height 1> /dev/null || die "Failed: response does not include main chain height"
api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query", "params": {"url": "keytest"}}' | jq -e .result.mainChain.roots 1> /dev/null
api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query", "params": {"url": "keytest"}}' | jq -e .result.mainChain.height 1> /dev/null
success

section "Query with txid and chainId (API v2, AC-602)"
# TODO Verify query-chain
TXID=$(accumulate -j tx history keytest 0 1 | jq -re '.items[0].txid')
GOT=$(api-v2 '{"jsonrpc": "2.0", "id": 0, "method": "query-tx", "params": {"txid": "'${TXID}'"}}' | jq -re .result.txid)
[ "${TXID}" = "${GOT}" ] || die "Failed to find TX ${TXID}"
success

section "Query transaction receipt"
TXID=$(accumulate -j tx history keytest 0 1 | jq -re '.items[0].txid')
(accumulate -j tx get --prove $TXID | jq -e .receipts[0] -C --indent 0) && success || die "Failed to get receipt for ${TXID}"

section "Create a token issuer"
wait-for cli-tx token create keytest keytest-1-0 keytest/token-issuer TOK 10
accumulate get keytest/token-issuer 1> /dev/null || die "Cannot find keytest/token-issuer"
success

section "Issue tokens"
LITE_TOK=$(echo $LITE | cut -d/ -f-3)/keytest/token-issuer
wait-for cli-tx token issue keytest/token-issuer keytest-1-0 ${LITE_TOK} 123.0123456789
BALANCE=$(accumulate -j account get ${LITE_TOK} | jq -r .data.balance)
[ "$BALANCE" -eq 1230123456789 ] && success || die "${LITE_TOK} should have 1230123456789 keytest tokens but has ${BALANCE}"

section "Add credits to lite account (TOK)"
wait-for cli-tx credits ${LITE} ${LITE_TOK} 100
BALANCE=$(accumulate -j account get ${LITE_TOK} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 100 ] && success || die "${LITE_TOK} should have at least 100 credits but only has ${BALANCE}"

section "Burn tokens"
wait-for cli-tx token burn ${LITE_TOK} 100
BALANCE=$(accumulate -j account get ${LITE_TOK} | jq -r .data.balance)
[ "$BALANCE" -eq 230123456789 ] && success || die "${LITE_TOK} should have 230123456789 keytest tokens but has ${BALANCE}"

section "Create lite data account and write the data"
ACCOUNT_ID=$(accumulate -j account create data --lite keytest keytest-1-0 "Factom PRO" "Tutorial" | jq -r .accountUrl)
[ "$ACCOUNT_ID" == "acc://b36c1c4073305a41edc6353a094329c24ffa54c029a521aa" ] || die "${ACCOUNT_ID} does not match expected value"
accumulate data get $ACCOUNT_ID 0 1 1> /dev/null || die "lite data entry not found"
wait-for cli-tx data write-to keytest keytest-1-0 $ACCOUNT_ID "data test"
accumulate data get $ACCOUNT_ID 0 2 1> /dev/null || die "lite data error"
accumulate -j get "${ACCOUNT_ID}#txn/0" | jq -re .status.result.entryHash &> /dev/null || die "Entry hash is missing from transaction results"
accumulate -j get "${ACCOUNT_ID}#txn/0" | jq -re .status.result.accountID &> /dev/null || die "Account ID is missing from transaction results"
success

section "Create ADI Data Account"
wait-for cli-tx account create data --scratch keytest keytest-1-0 keytest/data
accumulate account get keytest/data 1> /dev/null || die "Cannot find keytest/data"
accumulate -j account get keytest/data | jq -re .data.scratch 1> /dev/null || die "keytest/data is not a scratch account"
success

section "Write data to ADI Data Account"
JSON=$(accumulate -j data write keytest/data keytest-1-0 foo bar)
TXID=$(echo $JSON | jq -re .transactionHash)
echo $JSON | jq -C --indent 0
wait-for-tx $TXID
echo $JSON | jq -re .result.result.entryHash 1> /dev/null || die "Deliver response does not include the entry hash"
accumulate -j tx get $TXID | jq -re .status.result.entryHash 1> /dev/null || die "Transaction query response does not include the entry hash"
success

section "Create a sub ADI"
wait-for cli-tx adi create keytest keytest-1-0 keytest/sub1 keytest-2-0 keytest/sub1/book
accumulate adi get keytest/sub1 1> /dev/null && success || die "Cannot find keytest/sub1"

section "Add credits to the sub ADI's key page 0"
wait-for cli-tx credits ${LITE} keytest/sub1/book/1 60000
BALANCE=$(accumulate -j page get keytest/sub1/book/1 | jq -r .data.creditBalance)
[ "$BALANCE" -ge 60000 ] && success || die "keytest/sub1/book/1 should have 60000 credits but has ${BALANCE}"

section "Create Data Account for sub ADI"
wait-for cli-tx account create data --scratch keytest/sub1 keytest-2-0 keytest/sub1/data
accumulate account get keytest/sub1/data 1> /dev/null || die "Cannot find keytest/sub1/data"
accumulate -j account get keytest/sub1/data | jq -re .data.scratch 1> /dev/null || die "keytest/sub1/data is not a scratch account"
success

section "Write data to sub ADI Data Account"
JSON=$(accumulate -j data write keytest/sub1/data keytest-2-0 "foo" "bar")
TXID=$(echo $JSON | jq -re .transactionHash)
echo $JSON | jq -C --indent 0
wait-for-tx $TXID
echo $JSON | jq -re .result.result.entryHash 1> /dev/null || die "Deliver response does not include the entry hash"
accumulate -j tx get $TXID | jq -re .status.result.entryHash 1> /dev/null || die "Transaction query response does not include the entry hash"
success

section "Issue a new token"
JSON=$(accumulate -j token create keytest keytest-1-0 keytest/foocoin bar 8)
TXID=$(echo $JSON | jq -re .transactionHash)
echo $JSON | jq -C --indent 0
wait-for-tx $TXID
RESULT=$(accumulate -j token get keytest/foocoin)
[ "$(echo $RESULT | jq -re .data.symbol)" == "bar" ] || die "Token issuance failed with invalid symbol"
[ "$(echo $RESULT | jq -re .data.precision)" -eq 8 ] || die "Token issuance failed with invalid precision"
success

section "Query latest data entry by URL"
RESULT=$(accumulate -j get keytest/data#data | jq -re .data.entry.data)
[ "$RESULT" == $(echo -n bar | xxd -p) ] && success || die "Latest entry is not 'bar'"

section "Query data entry at height 0 by URL"
RESULT=$(accumulate -j get keytest/data#data/0 | jq -re .data.entry.data)
[ "$RESULT" == $(echo -n bar | xxd -p) ] && success || die "Entry at height 0 is not 'bar'"

section "Query data entry with hash by URL"
ENTRY=$(accumulate -j get keytest/data#data/0 | jq -re .data.entryHash)
RESULT=$(accumulate -j get keytest/data#data/${ENTRY} | jq -re .data.entry.data)
ENTRY2=$(accumulate -j get keytest/data#data/${ENTRY} | jq -re .data.entryHash)
[ "$RESULT" == $(echo -n bar | xxd -p) ] || die "Entry with hash ${ENTRY} is not 'bar'"
[ "$ENTRY" == "$ENTRY2" ] || die "Entry hash mismatch ${ENTRY} ${ENTRY2}"
success

section "Query data entry range by URL"
RESULT=$(accumulate -j get keytest/data#data/0:10 | jq -re .data.total)
[ "$RESULT" -ge 1 ] && success || die "No entries found"

section "Create keypage with manager"
wait-for cli-tx tx execute keytest/book keytest-1-0 '{"type": "createKeyPage","url": "keytest/page3", "manager": "keytest/book", "keys": [{"publicKey": "c8e1028cad7b105814d4a2e0e292f5f7904aad7b6cbc46a5"}]}'
RESULT=$(accumulate -j get keytest/page3 | jq -re .data.managerKeyBook)
[ "$RESULT" == "acc://keytest/book" ] && success || die "chain manager not set"

section "Update manager to keypage"
wait-for cli-tx manager set keytest/book/3 keytest-2-0 keytest/book
RESULT=$(accumulate -j get keytest/book/3 | jq -re .data.managerKeyBook)
[ "$RESULT" == "acc://keytest/book" ] && success || die "chain manager not set"

section "Remove manager from keypage"
wait-for cli-tx manager remove keytest/page3 keytest-2-0
accumulate -j get keytest/page3 | jq -re .data.managerKeyBook &> /dev/null && die "chain manager not removed" || success

section "Query the lite identity"
accumulate -s local get $(dirname $LITE) -j | jq -e -C --indent 0 .data && success || die "Failed to get $(dirname $LITE)"

section "Query the lite identity directory"
accumulate adi directory $(dirname $LITE) 0 10 1> /dev/null || die "Failed to get directory for $(dirname $LITE)"
TOTAL=$(accumulate -j adi directory $(dirname $LITE) 0 10 | jq -re .total)
[ "$TOTAL" -eq 2 ] && success || die "Expected directory 2 entries for $(dirname $LITE), got $TOTAL"

section "Create ADI Data Account with wait"
cli-tx account create data --scratch --wait 10s keytest keytest-1-0 keytest/data1
accumulate account get keytest/data1 1> /dev/null || die "Cannot find keytest/data1"

section "Query credits"
RESULT=$(accumulate -j oracle  | jq -re .price)
[ "$RESULT" -ge 0 ] && success || die "Expected 500, got $RESULT"

section "Query votes chain"
if [ -f "$NODE_PRIV_VAL" ]; then
    #xxd -r -p doesn't like the .data.entry.data hex string in docker bash for some reason, so converting using sed instead
    RESULT=$(accumulate -j data get dn/votes | jq -re .data.entry.data | sed 's/\([0-9A-F]\{2\}\)/\\\\\\x\1/gI' | xargs printf)
    #convert the node address to search for to base64
    NODE_ADDRESS=$(jq -re .address $NODE_PRIV_VAL | xxd -r -p | base64 )
    VOTE_COUNT=$(echo "$RESULT" | jq -re '.votes|length')
    FOUND=0
    for ((i = 0; i < $VOTE_COUNT; i++)); do
    R2=$(echo "$RESULT" | jq -re .votes[$i].validator.address)
    if [ "$R2" == "$NODE_ADDRESS" ]; then
        FOUND=1
    fi
    done
    [ "$FOUND" -eq  1 ] && success || die "No vote record found on DN"
else
    echo -e '\033[1;31mCannot verify the votes chain: private validator key not found\033[0m'
fi

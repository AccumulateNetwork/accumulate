#!/bin/bash

if [ -n "${DEBUG}" ] || [ -n "${DEBUG_JQ}" ]; then
    exec 3>&2
fi

# accumulate - Run CLI with default flags
function accumulate {
    [ -n "${DEBUG}" ] && echo -e '\033[36m'"[debug] accumulate $@"'\033[0m' >&3
    command accumulate --use-unencrypted-wallet --database="$HOME/.accumulate/validate" "$@"
}

function init-wallet {
    if [ -n "${MNEMONIC}" ]; then
        echo ${MNEMONIC} | accumulate wallet init import
    else
        accumulate wallet init script
    fi
}

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

# hash-from-txid - Extract the transaction hash from a transaction ID
function hash-from-txid {
    cut -d/ -f3 | cut -d@ -f1
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

# wait-for-tx [--no-check] [--ignore-pending] <tx id> - Wait for a transaction and any synthetic transactions to complete
function wait-for-tx {
    while true; do
        case "$1" in
        --no-check)
            local NO_CHECK=$1
            shift
            ;;
        --ignore-pending)
            local IGNORE_PENDING=$1
            shift
            ;;
        *)
            break
            ;;
        esac
    done

    local TXID=$1
    echo -e '\033[2mWaiting for '"$TXID"'\033[0m'
    local RESP=$(accumulate tx get -j $IGNORE_PENDING --wait 1m $TXID)
    echo $RESP | jq -C --indent 0

    if [ -z "$NO_CHECK" ]; then
        FAILED=$(echo $RESP | jq -re '.status.failed // "false"') || return 1
        [ "$FAILED" != "false" ] && die "$TXID failed:" $(echo $RESP | jq -C --indent 0 .status)
    fi

    for TXID in $(echo $RESP | jq -re '(.produced // [])[]' | hash-from-txid); do
        wait-for-tx $NO_CHECK --ignore-pending "$TXID" || return 1
    done
}

function cli-run {
    if ! JSON=`accumulate -j "$@" 2>&1`; then
        echo "$JSON" | jq -C --indent 0 >&2
        >&2 echo -e '\033[1;31m'"$@"'\033[0m'
        return 1
    fi
    echo "$JSON"
}

# cli-tx <args...> - Execute a CLI command and extract the transaction hash from the result
function cli-tx {
    JSON=`cli-run "$@"` || return 1
    echo "$JSON" | jq -re .transactionHash
}

# cli-tx-sig <args...> - Execute a CLI command and extract the first signature hash from the result
function cli-tx-sig {
    JSON=`cli-run "$@"` || return 1
    echo "$JSON" | jq -re .signatureHashes[0]
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
    >&2 echo -e '\033[1;31m'"$@"'\033[0m'
    exit 1
}

# success - Print 'success' in bold green text
function success {
    echo -e '\033[1;32m'Success'\033[0m'
    echo
}

# jq - Check for errors, then run JQ
function jq {
    local INPUT=$(cat)
    local ERROR

    [ -n "${DEBUG_JQ}" ] && echo -e '\033[36m'"[debug] jq $@ <<< $INPUT"'\033[0m' >&3

    if ERROR=$(command jq -re .error <<< "${INPUT}") && [ -n "$ERROR" ] && [[ $(command jq -re 'keys | length') -eq 1 ]] ; then
        [ -n "${DEBUG}" ] && echo -e '\033[36m'"[debug] error detected: $INPUT"'\033[0m' >&3
        command jq -C --indent 0 <<< "${INPUT}" 1>&2
        return 1
    fi

    command jq "$@" <<< "${INPUT}"
    return $?
}

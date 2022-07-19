#!/bin/bash

set -eu
set -x

# Make sure we're within the java directory
cd $( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

function generate {
    TOOL=../../tools/cmd/gen-$1
    FLAGS="--language java --package io.accumulatenetwork.sdk --out /media/pcRyzen/accumulate/accumulate-java-sdk/src/main/java/io/accumulatenetwork/sdk/generated/{{.Name}}.java"
    go run ${TOOL} ${FLAGS} "${@:2}"
}

PROT=../../protocol
generate enum $PROT/enums.yml
generate types $PROT/general.yml
generate types $PROT/account_auth_operations.yml $PROT/key_page_operations.yml $PROT/signatures.yml $PROT/transaction_results.yml $PROT/transaction.yml $PROT/user_transactions.yml \
    -x TransactionStatus

MAN=../../smt/managed
generate enum $MAN/enums.yml
generate types $MAN/types.yml

API=../../internal/api/v2
generate types $API/types.yml
generate api $API/methods.yml

QRY=../../types/api/query
generate enum $QRY/enums.yml
generate types $QRY/responses.yml

CFG=../../config
generate enum $CFG/enums.yml
generate types $CFG/types.yml

CORE=../../internal/core
generate types $CORE/types.yml

ERR=../../internal/errors
generate enum $ERR/status.yml
generate types $ERR/error.yml


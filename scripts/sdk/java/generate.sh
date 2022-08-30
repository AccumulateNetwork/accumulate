#!/bin/bash

set -eu
set -x

if [ -z "$1" ]; then
    echo "Usage: $0 <out-dir>"
    exit 1
fi

mkdir -p $1
declare outDir="$( cd -- "$1" &> /dev/null && pwd )"

# Make sure we're within the java directory
cd $( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )


function generate {
    TOOL=../../../tools/cmd/gen-$1
    mkdir -p $outDir/$2
    FLAGS="--language java --package io.accumulatenetwork.sdk --subpackage $2 --out $outDir/{{.SubPackage}}/{{.Name}}.java"
    go run ${TOOL} ${FLAGS} "${@:3}"
}

PROT=../../../protocol
generate enum protocol $PROT/enums.yml
generate types protocol $PROT/general.yml
generate types protocol $PROT/account_auth_operations.yml $PROT/key_page_operations.yml $PROT/signatures.yml $PROT/transaction_results.yml $PROT/transaction.yml \
    $PROT/user_transactions.yml $PROT/synthetic_transactions.yml $PROT/accounts.yml \
    -x TransactionStatus

MAN=../../../smt/managed
generate enum managed $MAN/enums.yml
generate types managed $MAN/types.yml

API=../../../internal/api/v2
generate types apiv2 $API/types.yml
generate api apiv2 $API/methods.yml

QRY=../../../internal/api/v2/query
generate enum query $QRY/enums.yml
generate types query $QRY/requests.yml
generate types query $QRY/responses.yml

CFG=../../../config
generate enum config $CFG/enums.yml
generate types config $CFG/types.yml

CORE=../../../internal/core
generate types core $CORE/types.yml

ERR=../../../internal/errors
generate enum errors $ERR/status.yml
generate types errors $ERR/error.yml


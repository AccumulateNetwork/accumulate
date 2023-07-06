#!/bin/bash

set -eu

if [ $# -eq 0 ]; then
    echo "Usage: $0 <out-dir>"
    exit 1
fi

set -x


mkdir $1
declare outDir="$( cd -- "$1" &> /dev/null && pwd )"

# Make sure we're within the java directory
cd $( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )


function generate {
    TOOL=../../../tools/cmd/gen-$1
    mkdir -p $outDir/$2
    FLAGS="--language kotlin --package io.accumulatenetwork.sdk --subpackage $2 --out $outDir/{{.SubPackage}}/{{.Name}}.kt"
    go run ${TOOL} ${FLAGS} "${@:3}"
}

PROT=../../../protocol
generate enum protocol $PROT/enums.yml
generate types protocol $PROT/general.yml $PROT/account_auth_operations.yml $PROT/key_page_operations.yml $PROT/accounts.yml $PROT/signatures.yml $PROT/transaction_results.yml $PROT/transaction.yml \
    $PROT/user_transactions.yml $PROT/synthetic_transactions.yml $PROT/query.yml \
    -x TransactionStatus

TYPES=../../../pkg/types
generate types types $TYPES/merkle/types.yml $TYPES/network/types.yml

API=../../../pkg/api/v3
generate enum apiv3 $API/enums.yml
generate types apiv3 $API/responses.yml $API/options.yml $API/records.yml $API/events.yml $API/types.yml $API/queries.yml --reference $TYPES/merkle/types.yml,$PROT/general.yml

MSG=../../../pkg/api/v3/message
generate enum message $MSG/enums.yml --registry Type:messageRegistry
generate types message --elide-package-type $MSG/messages.yml $MSG/private.yml --reference $API/options.yml


ERR=../../../pkg/errors
generate enum errors $ERR/status.yml
generate types errors $ERR/error.yml


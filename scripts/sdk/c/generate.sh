#!/bin/bash

set -eu
set -x

if [ -z "$1" ]; then
    echo "Usage: $0 <out-dir>"
    exit 1
fi

mkdir $1
declare outDir="$( cd -- "$1" &> /dev/null && pwd )"

# Make sure we're within the java directory
cd $( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
function generate {
    TOOL=../../../tools/cmd/gen-$1
    mkdir -p $outDir/include/accumulate/$3
    FLAGS="--language $2-header --package io.accumulatenetwork.sdk --subpackage $3"
    go run ${TOOL} ${FLAGS} --out "$outDir/include/accumulate/{{.SubPackage}}/{{snake .Name}}.h" "${@:4}"

    mkdir -p $outDir/src/accumulate/$3/generated
    FLAGS="--language $2-source --package io.accumulatenetwork.sdk --subpackage $3"
    go run ${TOOL} ${FLAGS} --out "$outDir/src/accumulate/{{.SubPackage}}/generated/{{snake .Name}}_gen.c" "${@:4}"
}

PROT=../../../protocol
SMT=../../../internal/database/smt/managed
generate enum c protocol $PROT/enums.yml # $SMT/enums.yml
generate types c protocol  $PROT/account_auth_operations.yml $PROT/accounts.yml $PROT/general.yml $PROT/system.yml $PROT/key_page_operations.yml $PROT/query.yml $PROT/signatures.yml  $PROT/synthetic_transactions.yml $PROT/transaction.yml $PROT/transaction_results.yml $PROT/user_transactions.yml  -x TransactionStatus -x SystemLedger  -x TransactionResultSet
#    -x TransactionStatus



#MAN=../../../smt/managed
#generate enum c managed $MAN/enums.yml
#generate types c managed $MAN/types.yml

#API=../../../internal/api/v2
#generate types apiv2 $API/types.yml
#generate api apiv2 $API/methods.yml

#QRY=../../../internal/api/v2/query
#generate enum query $QRY/enums.yml
#generate types query $QRY/requests.yml
#generate types query $QRY/responses.yml

#CFG=../../../config
#generate enum c config $CFG/enums.yml
#generate types c config $CFG/types.yml

#CORE=../../../internal/core
#generate types c core $CORE/types.yml

ERR=../../../pkg/errors
generate enum c errors $ERR/status.yml
generate types c errors $ERR/error.yml

#for i in `ls -1 $1/include/protocol`; do echo "#include <protocol/$i>" >> $1/include/protocol/types.h; done

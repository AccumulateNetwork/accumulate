#!/bin/bash

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <out-dir>"
    exit 1
fi

mkdir -p $1
declare outDir="$( cd -- "$1" &> /dev/null && pwd )"

# Make sure we're within the java directory
cd $( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

to_pascal() {
    local input="$1"
    echo "$input" | sed -E 's/(^|\/)([a-z])/\1\u\2/g; s/[_-]//g'
}

function generate {
    echo "generate ${@:1:2}"
    TOOL=../../../tools/cmd/gen-$1
    dest=$(to_pascal $2)
    mkdir -p $outDir/$dest
    FLAGS="--language csharp --package AccumulateNetwork.SDK --subpackage $dest --out $outDir/{{.SubPackage}}/{{.Name}}.cs"
    go run ${TOOL} ${FLAGS} "${@:3}"
}

PROT=../../../protocol
echo "Generating Enums..."
generate enum Protocol $PROT/enums.yml
echo "Generating Types for general..."
generate types Protocol $PROT/general.yml
echo "Generating Types for ooperations..."
generate types Protocol $PROT/operations.yml $PROT/accounts.yml $PROT/general.yml $PROT/system.yml $PROT/key_page_operations.yml $PROT/query.yml $PROT/signatures.yml  $PROT/synthetic_transactions.yml $PROT/transaction.yml $PROT/transaction_results.yml $PROT/user_transactions.yml  -x TransactionStatus -x SystemLedger  -x TransactionResultSet

#generate types Protocol $PROT/operations.yml $PROT/key_page_operations.yml $PROT/signatures.yml $PROT/transaction_results.yml $PROT/transaction.yml \
#    $PROT/user_transactions.yml $PROT/synthetic_transactions.yml $PROT/accounts.yml \
#    -x TransactionStatus

MAN=../../../smt/managed
#echo "Generating Enums..."
#generate enum Managed $MAN/enums.yml
#echo "Generating Types..."
#generate types Managed $MAN/types.yml

echo "Generating Apiv2 types..."
API=../../../internal/api/v2
generate enum Apiv2 $API/enums.yml
generate types Apiv2 $API/types.yml $API/responses.yml
echo "Generating Apiv2 api methods..."
generate api Apiv2 $API/methods.yml

QRY=../../../internal/api/v2/query
echo "Generating Query enumds..."
#generate enum Query $QRY/enums.yml
#generate types Query $QRY/requests.yml
#generate types Query $QRY/responses.yml

CFG=../../../config
echo "Generating Config enums and types..."
generate enum Config $CFG/enums.yml
generate types Config $CFG/types.yml

CORE=../../../internal/core
echo "Generating core types..."
#generate types Core $CORE/types.yml

ERR=../../../pkg/errors
generate enum Errors $ERR/status.yml
generate types Errors $ERR/error.yml


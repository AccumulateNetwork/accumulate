#!/bin/bash

set -eu

# Make sure we're within the java directory
cd $( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

function generate {
    TOOL=../../tools/cmd/gen-$1
    FLAGS="--language java --package io.accumulatenetwork.accumulate --out {{.Name}}.java"
    go run ${TOOL} ${FLAGS} "${@:2}"
}

P=../../protocol
generate enum $P/enums.yml \
    -i TransactionType,SignatureType,DataEntryType,KeyPageOperationType,AccountAuthOperationType,VoteType
generate types $P/general.yml \
    -i AccumulateDataEntry,FactomDataEntry,TokenRecipient,KeySpecParams
generate types $P/account_auth_operations.yml $P/key_page_operations.yml $P/signatures.yml $P/transaction_results.yml $P/transaction.yml $P/user_transactions.yml \
    -x TransactionStatus
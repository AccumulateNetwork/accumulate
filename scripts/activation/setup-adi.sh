#!/bin/bash

set -e

function die {
    >&2 echo -e '\033[1;31m'"$@"'\033[0m'
    exit 1
}

function accumulate {
    >&2 echo "accumulate $@"
    command accumulate --database ~/.accumulate/activation "$@"
}

ADI="$1"
[ -z "$ADI" ] && die "Usage: $0 <adi>"
ADI="$(accumulate get $ADI -j | jq -r .data.url | cut -d/ -f3-)"

OWNER=$(accumulate get $ADI -j | jq -re '.data.authorities[0].url' | cut -d/ -f3-)
LITE=acc://33bc5c1bfcf4dab98ce1dc2556320254e52b7e60075ee7b0/ACME

[[ $(echo $OWNER | cut -d/ -f1) == "$ADI" ]] && die "Nothing to do"

echo $PW | accumulate book create $ADI activation@$OWNER $ADI/book activation --wait 10s; echo
echo $PW | accumulate credits $LITE $ADI/book/1 2000 --wait 10s; echo
JSON=$(echo $PW | accumulate tx execute $ADI activation@$OWNER '{ type: updateAccountAuth, operations: [ { type: addAuthority, authority: '$ADI'/book }, { type: removeAuthority, authority: '$OWNER' } ] }' -j); echo
echo $PW | accumulate tx get $(jq -re .transactionHash <<< $JSON) --wait 10s; echo
echo $PW | accumulate tx sign $ADI activation@$ADI/book $(jq -re .transactionHash <<< $JSON); echo
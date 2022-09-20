#!/bin/bash

# Stop immediately on error
set -e

SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
source "${SCRIPT_DIR}/validate-commons.sh"
section "Setup"
if which go > /dev/null || ! which accumulate > /dev/null ; then
    echo "Installing CLI"
    go install ./cmd/accumulate
    export PATH="${PATH}:$(go env GOPATH)/bin"
fi
init-wallet
echo

section "Show ledgers info"
accumulate ledger info -j
STATUS=$(accumulate -j ledger info | jq -re '.[0].status')
[ "$STATUS" == "ok" ] && success || die "Ledger status is not ok: $STATUS"
success

section "Generate a derived key"
PUBLIC_KEY=$(accumulate -j ledger key generate ledgerkey1 | jq -re '.publicKey')
[ ${#PUBLIC_KEY} -eq 64 ] || die "Public key is not 32 bytes long: $PUBLIC_KEY"
echo "PUBLIC_KEY: $PUBLIC_KEY"
success

section "Create & fund a Lite Token Account from the new key"
LITE_ACME=$(accumulate account list -j | jq -re .liteAccounts[0].liteAccount)
LITE_ID=$(cut -d/ -f-3 <<< "$LITE_ACME")
wait-for cli-tx  faucet ${LITE_ACME}
accumulate account get ${LITE_ACME} 1> /dev/null && success || die "Cannot find ${LITE_ACME}"

section "Create a receiver Lite Token Account"
# TODO

section "Send tokens from the ledger token account to the recever token account"
wait-for cli-tx tx create ${LITE_ACME} ledgertest.acme/tokens 5
BALANCE=$(accumulate -j account get ledgertest.acme/tokens | jq -r .data.balance)
[ "$BALANCE" -eq 500000000 ] && success || die "${LITE_ACME} should have 5 tokens but has $(expr ${BALANCE} / 100000000)"



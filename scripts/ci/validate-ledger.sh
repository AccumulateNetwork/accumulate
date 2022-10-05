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
success

section "Create & fund a Lite Token Account from the new key"
LITE_ACME=$(accumulate account list -j | jq -re .liteAccounts[0].liteAccount)
LITE_ID=$(cut -d/ -f-3 <<< "$LITE_ACME")
wait-for cli-tx faucet ${LITE_ACME}
accumulate account get ${LITE_ACME} 1> /dev/null && success || die "Cannot find ${LITE_ACME}"
wait-for cli-tx credits ${LITE_ACME} ${LITE_ID} 5


section "Create a receiver Lite Token Account"
accumulate account generate
RECV_LITE_ACME=$(accumulate account list -j | jq -re .liteAccounts[0].liteAccount)
RECV_LITE_ID=$(cut -d/ -f-3 <<< "$RECV_LITE_ACME")
TXS=()
for i in {1..1}
do
	TXS=(${TXS[@]} $(cli-tx faucet ${RECV_LITE_ACME}))
done
for tx in "${TXS[@]}"
do
	echo $tx
	wait-for-tx $tx
done

accumulate account get ${RECV_LITE_ACME} 1> /dev/null && success || die "Cannot find ${RECV_LITE_ACME}"
echo receiver balance before send tokens:
accumulate account get ${RECV_LITE_ACME}
accumulate account get ${RECV_LITE_ID}


section "Send tokens from the ledger lite token account to the receiver lite token account"
TXID=$(cli-tx tx create ${LITE_ACME} ${RECV_LITE_ACME} 2)
wait-for-tx $TXID
accumulate -j tx get $TXID | jq -re .status.pending 1> /dev/null || die "Transaction is not pending"
accumulate -j tx get $TXID | jq -re .status.delivered 1> /dev/null && die "Transaction was delivered"
success
echo liteid balance:
accumulate account get ${LITE_ACME}
echo receiver balance after send tokens:
accumulate account get ${RECV_LITE_ACME}

section "Send credits from the ledger lite token account to the receiver lite ID"
wait-for cli-tx credits ${LITE_ACME} ${RECV_LITE_ID} 5
echo liteid balance:
accumulate account get ${LITE_ACME}
echo receiver balance after send tokens:
accumulate account get ${RECV_LITE_ID}



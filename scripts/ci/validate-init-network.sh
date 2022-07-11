
#!/bin/bash

tmpNodeDir=$(mktemp -d -t accumulated-ci-XXXXXXXX)
accumulated init network data/devnet.json --no-website -w ${tmpNodeDir} --no-empty-blocks
accumulated run-dual "$tmpNodeDir/node-1/dnn" "$tmpNodeDir/node-1/bvnn" &
declare -g ACCPID=$!

section "Generate a Lite Token Account"
accumulate account list 2>&1 | grep -q ACME || accumulate account generate
LITE_ACME=$(accumulate account list -j | jq -re .liteAccounts[0].liteAccount)
LITE_ID=$(cut -d/ -f-3 <<< "$LITE_ACME")
TXS=()
for i in {1..1}
do
	TXS=(${TXS[@]} $(cli-tx faucet ${LITE_ACME}))
done
for tx in "${TXS[@]}"
do
	echo $tx
	wait-for-tx $tx
done

accumulate account get ${LITE_ACME} 1> /dev/null && success || die "Cannot find ${LITE_ACME}"

section "Add credits to lite account"
TXID=$(cli-tx credits ${LITE_ACME} ${LITE_ID} 2700)
wait-for-tx $TXID
BALANCE=$(accumulate -j account get ${LITE_ID} | jq -r .data.creditBalance)
[ "$BALANCE" -ge 2700 ] || die "${LITE_ID} should have at least 2700 credits but only has ${BALANCE}"
success


kill -9 $ACCPID || true
rm -rf "$tmpNodeDir"

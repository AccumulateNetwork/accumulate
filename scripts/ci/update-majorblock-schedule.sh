#!/bin/bash

# Stop immediately on error
set -e

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${SCRIPT_DIR}"/validate-commons.sh

# Get number of signatures required using N of M factor
function sigCount {
  echo $(printf %.$2f $(echo $(bc -l <<<"($NUM_DNNS * $M_OF_N_FACTOR) + 0.5")))
}

# Format the path to priv_validator_key.json
function nodePrivKey {
  echo $DN_NODES_DIR/Node$1/config/priv_validator_key.json
}

declare -r M_OF_N_FACTOR=$(bc -l <<<'2/3')
declare -g NUM_DNNS=$(find ${DN_NODES_DIR} -mindepth 1 -maxdepth 1 -type d | wc -l)

section "Set major block time to 1 minute"
if [ -f "$(nodePrivKey 0)" ]; then
  TXID=$(cli-tx data write dn/globals "$(nodePrivKey 0)" '{"validatorThreshold":{"numerator":2,"denominator":3},"majorBlockSchedule":"* * * * *"}')
  wait-for-tx $TXID

  # Sign the required number of times
  for ((sigNr = 1; sigNr < $(sigCount); sigNr++)); do
    wait-for cli-tx-sig tx sign dn/validators "$(nodePrivKey $sigNr)" $TXID
  done
  accumulate --use-unencrypted-wallet -j tx get $TXID | jq -re .status.pending 1>/dev/null && die "Transaction is pending"
  accumulate --use-unencrypted-wallet -j tx get $TXID | jq -re .status.delivered 1>/dev/null || die "Transaction was not delivered"
else
  echo -e '\033[1;31mCannot update globals: private validator key not found\033[0m'
  echo
fi

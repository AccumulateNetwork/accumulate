#!/bin/bash

# Stop immediately on error
set -e

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${SCRIPT_DIR}"/validate-commons.sh

# Format the path to priv_validator_key.json
function dnPrivKey {
  echo $NODES_DIR/dn/Node$1/config/priv_validator_key.json
}

function signKey {
      if [ "$1" -lt "$NUM_NODES" ]; then
        echo "$NODES_DIR/dn/Node$1/config/priv_validator_key.json"
      else
        declare -r SK_BVN_NR=$(bc -l <<<"scale=0;($1/$NUM_NODES)-1")
        declare -r SK_BVN_NODE_NR=$(bc -l <<<"$1-(($SK_BVN_NR+1) * ($NUM_SUBNETS-1))")
        echo "$NODES_DIR/bvn$SK_BVN_NR/Node$SK_BVN_NODE_NR/config/priv_validator_key.json"
      fi
}

function signCount {
   echo "$(bc -l <<<"$ACCEPT_THRESHOLD-1")"
}


declare -g NUM_SUBNETS=$(find ${NODES_DIR} -mindepth 1 -maxdepth 1 -type d | wc -l)
declare -g NUM_NODES=$(find ${NODES_DIR}/dn -mindepth 1 -maxdepth 1 -type d | wc -l)
declare -g ACCEPT_THRESHOLD=$(accumulate page get -j dn.acme/operators/1 | jq -re .data.acceptThreshold)

section "Set major block time to 1 minute"
if [ -f "$(dnPrivKey 0)" ]; then
  TXID=$(cli-tx data write dn.acme/globals "$(dnPrivKey 0)" '{"validatorThreshold":{"numerator":2,"denominator":3},"majorBlockSchedule":"* * * * *"}')
  wait-for-tx $TXID

  # Sign the required number of times
  echo Signature count $(signCount)
  for ((sigNr = 1; sigNr <= $(signCount); sigNr++)); do
    echo Signature $sigNr
    wait-for cli-tx-sig tx sign dn.acme/operators "$(signKey $sigNr)" $TXID
  done
  accumulate -j tx get $TXID | jq -re .status.pending 1>/dev/null && die "Transaction is pending"
  accumulate -j tx get $TXID | jq -re .status.delivered 1>/dev/null || die "Transaction was not delivered"
else
  echo -e '\033[1;31mCannot update globals: private validator key not found\033[0m'
  echo
fi

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

section "Setup"
if which go >/dev/null || ! which accumulate >/dev/null; then
  echo "Installing CLI & daemon"
  go install ./cmd/accumulate
  go install ./cmd/accumulated
  export PATH="${PATH}:$(go env GOPATH)/bin"
fi
[ -z "${MNEMONIC}" ] || accumulate key import mnemonic ${MNEMONIC}
echo

declare -r M_OF_N_FACTOR=$(bc -l <<<'2/3')
declare -g NUM_DNNS=$(find ${DN_NODES_DIR} -mindepth 1 -maxdepth 1 -type d | wc -l)
#spin up a DN validator, we cannot have 2 validators, so need >= 3 to run this test
if [ -f "$(nodePrivKey 0)" ] && [ -f "/.dockerenv" ] && [ "$NUM_DNNS" -ge "3" ]; then
  section "Add a new DN validator"

  # NUM_DNNS already contains the next node number (which starts counting at 0)
  accumulated init node "$NUM_DNNS" tcp://dn-0:26656 --listen=tcp://127.0.1.100:26656 -w "$DN_NODES_DIR" --genesis-doc="${DN_NODES_DIR}/Node0/config/genesis.json" --skip-version-check --no-website

  pubkey=$(jq -re .pub_key.value <"$(nodePrivKey $NUM_DNNS)")
  pubkey=$(echo $pubkey | base64 -d | od -t x1 -An)
  declare -g hexPubKey=$(echo $pubkey | tr -d ' ')

  # Register new validator
  TXID=$(cli-tx validator add dn.acme/validators/2 "$(nodePrivKey 0)" $hexPubKey)
  wait-for-tx $TXID

  # Sign the required number of times
  for ((sigNr = 1; sigNr < $(sigCount); sigNr++)); do
    wait-for cli-tx-sig tx sign dn.acme/operators "$(nodePrivKey $sigNr)" $TXID
  done

  # Start the new validator and increment NUM_DMNS
  accumulated run -n 3 -w "$DN_NODES_DIR" &
  declare -g ACCPID=$!

  # Increment NUM_DNNS so sigCount returns an updated result
  #  declare -g NUM_DNNS=$((NUM_DNNS + 1))  The node is initialized from a genesis doc, the key for th new node is not in there so we won't get an additional key in the validator book

  echo Updated keypage dn.acme/validators/2
  accumulate --use-unencrypted-wallet page get acc://dn.acme/validators/2
fi

section "Add a key to the operator book"
if [ -f "$(nodePrivKey 0)" ]; then

  wait-for cli-tx page key add acc://dn.acme/operators/1 "$(nodePrivKey 0)" $hexPubKey
  KEY_ADDED_DN=$(accumulate --use-unencrypted-wallet page get -j dn.acme/operators/1) | jq -re .data.keys[2].publicKey
  echo "sleeping for 5 seconds (wait for anchor)"
  sleep 5
  KEY_ADDED_BVN=$(accumulate --use-unencrypted-wallet page get -j bvn-BVN0.acme/operators/2) | jq -re .data.keys[2].publicKey
  [[ $KEY_ADDED_DN == $KEY_ADDED_BVN ]] || die "operator-2 was not sent to the BVN"

  echo Current keypage dn.acme/validators/2
  accumulate --use-unencrypted-wallet page get acc://dn.acme/validators/2
  echo Current keypage dn.acme/operators/1
  accumulate --use-unencrypted-wallet page get acc://dn.acme/operators/1
else
  echo -e '\033[1;31mCannot test the operator book: private validator key not found\033[0m'
  echo
fi

section "Update oracle price to \$0.0501. Oracle price has precision of 4 decimals"
if [ -f "$(nodePrivKey 0)" ]; then
  TXID=$(accumulated set oracle 0.0501 -w "${DN_NODES_DIR}/Node0" | grep Hash | cut -d: -f2)
  wait-for-tx $TXID

  # Sign the required number of times
  for ((sigNr = 1; sigNr < $(sigCount); sigNr++)); do
    echo sigNr: "$sigNr"
    wait-for cli-tx-sig tx sign dn.acme/operators "$(nodePrivKey $sigNr)" $TXID
  done
  accumulate -j tx get $TXID | jq -re .status.pending 1>/dev/null && die "Transaction is pending"
  accumulate -j tx get $TXID | jq -re .status.delivered 1>/dev/null || die "Transaction was not delivered"

  RESULT=$(accumulate --use-unencrypted-wallet -j oracle  | jq -re .price)
  [ "$RESULT" == "501" ] && success || die "cannot update price oracle"
else
  echo -e '\033[1;31mCannot update oracle: private validator key not found\033[0m'
  echo
fi

section "Query votes chain"
if [ -f "$(nodePrivKey 0)" ]; then
  #xxd -r -p doesn't like the .data.entry.data hex string in docker bash for some reason, so converting using sed instead
  RESULT=$(accumulate -j data get dn.acme/votes | jq -re .data.entry.data[0] | sed 's/\([0-9A-F]\{2\}\)/\\\\\\x\1/gI' | xargs printf)
  #convert the node address to search for to base64
  NODE_ADDRESS=$(jq -re .address "$(nodePrivKey 0)" | xxd -r -p | base64)
  VOTE_COUNT=$(echo "$RESULT" | jq -re '.votes|length')
  FOUND=0
  for ((i = 0; i < $VOTE_COUNT; i++)); do
    R2=$(echo "$RESULT" | jq -re .votes[$i].validator.address)
    if [ "$R2" == "$NODE_ADDRESS" ]; then
      FOUND=1
    fi
  done
  [ "$FOUND" -eq 1 ] && success || die "No vote record found on DN"
else
  echo -e '\033[1;31mCannot verify the votes chain: private validator key not found\033[0m'
fi

if [ ! -z "${ACCPID}" ]; then
  section "Shutdown dynamic validator"
  TXID=$(cli-tx validator remove dn.acme/validators/2 "$(nodePrivKey 0)" "$(nodePrivKey 3)")
  wait-for-tx $TXID

  # Sign the required number of times
  for ((sigNr = 1; sigNr < $(sigCount); sigNr++)); do
    wait-for cli-tx-sig tx sign dn.acme/operators "$(nodePrivKey $sigNr)" $TXID
  done
  accumulate -j tx get $TXID | jq -re .status.pending 1>/dev/null && die "Transaction is pending"
  accumulate -j tx get $TXID | jq -re .status.delivered 1>/dev/null || die "Transaction was not delivered"

  kill -9 $ACCPID || true
fi

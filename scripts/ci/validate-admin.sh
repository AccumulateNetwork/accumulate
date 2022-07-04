#!/bin/bash

# Stop immediately on error
set -e

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${SCRIPT_DIR}"/validate-commons.sh
# Format the path to priv_validator_key.json
function dnPrivKey {
  echo "$NODES_DIR/node-${1}/priv_validator_key.json"
}

function signCount {
  echo "$(bc -l <<<"$ACCEPT_THRESHOLD")"
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

declare -g NUM_NODES=$(find ${NODES_DIR} -mindepth 1 -maxdepth 1 -type d | wc -l)
declare -g ACCEPT_THRESHOLD=$(accumulate page get -j dn.acme/operators/1 | jq -re .data.acceptThreshold)

#spin up a DN validator, we cannot have 2 validators, so need >= 3 to run this test
if [ -f "$(dnPrivKey 1)" ] && [ -f "/.dockerenv" ] && [ "$NUM_NODES" -ge "3" ]; then
  section "Add a new DN validator"

  ((NUM_NODES++))
  accumulated init node tcp://node-1:26656 --listen=tcp://127.0.1.100:26656 -w "$NODES_DIR" --genesis-doc="${NODES_DIR}/node-1/dnn/config/genesis.json" --skip-version-check --no-website --skip-peer-health-check

  pubkey=$(jq -re .pub_key.value <"$(dnPrivKey $NUM_NODES)")
  pubkey=$(echo $pubkey | base64 -d | od -t x1 -An)
  declare -g hexPubKey=$(echo $pubkey | tr -d ' ')

  echo Current keypage dn.acme/operators/1
  accumulate page get acc://dn.acme/operators/1 -j

  # Add key to operator book first
  echo operator add dn "$(dnPrivKey 1)" $hexPubKey
  TXID=$(cli-tx operator add dn "$(dnPrivKey 1)" $hexPubKey)
  wait-for-tx $TXID
  # Sign the required number of times
  for ((sigNr = 2; sigNr <= $(signCount); sigNr++)); do
    wait-for cli-tx-sig tx sign dn.acme/operators "$(dnPrivKey $sigNr)" $TXID
  done
  declare -g ACCEPT_THRESHOLD=$(accumulate page get -j dn.acme/operators/1 | jq -re .data.acceptThreshold)

  # Register new validator
  echo validator add dn "$(dnPrivKey 1)" $hexPubKey
  TXID=$(cli-tx validator add dn "$(dnPrivKey 1)" $hexPubKey)
  wait-for-tx $TXID

  # Sign the required number of times
  echo Signature count $(signCount)
  for ((sigNr = 2; sigNr <= $(signCount); sigNr++)); do
    echo Signature $sigNr
    wait-for cli-tx-sig tx sign dn.acme/operators "$(dnPrivKey $sigNr)" $TXID
  done

  # Start the new validator and increment NUM_DMNS
  accumulated run -n 3 -w "$NODES_DIR/nodes-1/ndn" &
  declare -g ACCPID=$!
fi

section "Add a key to the operator book"
if [ -f "$(dnPrivKey 1)" ]; then
  DN_NEW_KEY="4a4557cfe5fe2c1e92f1ca91d0d78fe3c7f34a1a754a5084e7f743dbe7ac5ccd"
  DN_NEW_KEY_HASH="a8997980d7a4325b30f371d877daba11ae2a0b3ffb2edf0f3ebee5134460bac0"
  echo operator add dn "$(dnPrivKey 1)" $DN_NEW_KEY
  TXID=$(cli-tx operator add dn "$(dnPrivKey 1)" $DN_NEW_KEY)
  wait-for-tx $TXID

  # Sign the required number of times
  echo Signature count $(signCount)
  for ((sigNr = 2; sigNr <= $(signCount); sigNr++)); do
    echo Signature $sigNr
    wait-for cli-tx-sig tx sign dn.acme/operators "$(dnPrivKey $sigNr)" $TXID
  done

  echo "sleeping for 10 seconds (wait for anchor)"
  sleep 10
  KEY_ADDED_BVN=$(accumulate page get bvn-BVN1.acme/operators/1 | grep $DN_NEW_KEY_HASH || true)
  [[ -z $KEY_ADDED_BVN ]] && die "operator-2 was not sent to the BVN"

  declare -g ACCEPT_THRESHOLD=$(accumulate page get -j dn.acme/operators/1 | jq -re .data.acceptThreshold)
else
  echo -e '\033[1;31mCannot test the operator book: private validator key not found\033[0m'
  echo
fi

section "Update oracle price to \$0.0501. Oracle price has precision of 4 decimals"
if [ -f "$(dnPrivKey 1)" ]; then
  TXID=$(accumulated set oracle 0.0501 -w "${NODES_DIR}/node-1/dnn" | grep Hash | cut -d: -f2)
  wait-for-tx $TXID

  # Sign the required number of times
  echo Signature count $(signCount)
  for ((sigNr = 2; sigNr <= $(signCount); sigNr++)); do
    echo Signature $sigNr
    wait-for cli-tx-sig tx sign dn.acme/operators "$(dnPrivKey $sigNr)" $TXID
  done
  accumulate -j tx get $TXID | jq -re .status.pending 1>/dev/null && die "Transaction is pending"
  accumulate -j tx get $TXID | jq -re .status.delivered 1>/dev/null || die "Transaction was not delivered"

  RESULT=$(accumulate --use-unencrypted-wallet -j oracle | jq -re .price)
  [ "$RESULT" == "501" ] && success || die "cannot update price oracle"
else
  echo -e '\033[1;31mCannot update oracle: private validator key not found\033[0m'
  echo
fi


section "Query votes chain"
if [ -f "$(dnPrivKey 1)" ]; then
  #xxd -r -p doesn't like the .data.entry.data hex string in docker bash for some reason, so converting using sed instead
  RESULT=$(accumulate -j data get dn.acme/votes | jq -re .data.entry.data[0] | sed 's/\([0-9A-F]\{2\}\)/\\\\\\x\1/gI' | xargs printf)
  #convert the node address to search for to base64
  NODE_ADDRESS=$(jq -re .address "$(dnPrivKey 1)" | xxd -r -p | base64)
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
  TXID=$(cli-tx validator remove dn "$(dnPrivKey 1)" "$hexPubKey")
  wait-for-tx $TXID

  # Sign the required number of times
  echo Signature count $(signCount)
  for ((sigNr = 2; sigNr <= $(signCount); sigNr++)); do
    echo Signature $sigNr
    wait-for cli-tx-sig tx sign dn.acme/operators "$(dnPrivKey $sigNr)" $TXID
  done
  accumulate -j tx get $TXID | jq -re .status.pending 1>/dev/null && die "Transaction is pending"
  accumulate -j tx get $TXID | jq -re .status.delivered 1>/dev/null || die "Transaction was not delivered"

  kill -9 $ACCPID || true
fi

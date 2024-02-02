#!/bin/bash
# image: registry.gitlab.com/accumulatenetwork/accumulate:v1-2-12

set -e

function update-config {
    awk '{gsub(/^max_tx_bytes = 1048576$/, "max_tx_bytes = 4194304");print$0}' $1 > /tmp/file
    cat /tmp/file > $1
}

node="$1"
if [ -z "$node" ]; then
  node=/node
fi
if ! [ -d "$node" ]; then
  >&2 echo "Error: $node is not a directory"
  >&2 echo "Usage: $0 [node dir]"
  exit 1
fi

update-config "$node/dnn/config/tendermint.toml"
update-config "$node/bvnn/config/tendermint.toml"

grep -i max_tx_bytes "$node/dnn/config/tendermint.toml" "$node/bvnn/config/tendermint.toml"

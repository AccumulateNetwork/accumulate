#!/bin/bash
# image: registry.gitlab.com/accumulatenetwork/accumulate:v1-2-11

set -e

function update-max-env {
    if `grep -q '^max-envelopes-per-block = ' "$1"`; then
        awk '{gsub(/^max-envelopes-per-block = .*$/, "max-envelopes-per-block = 100");print$0}' "$1" > /tmp/file
    else
        { echo 'max-envelopes-per-block = 100'; cat "$1"; } > /tmp/file
    fi
    cat /tmp/file > "$1"
}

function update-mempool {
    awk '{gsub(/^size = 10000$/, "size = 5000");print$0}' $1 > /tmp/file
    cat /tmp/file > $1
    awk '{gsub(/^size = 15000$/, "size = 5000");print$0}' $1 > /tmp/file
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

update-max-env "$node/dnn/config/accumulate.toml"
update-max-env "$node/bvnn/config/accumulate.toml"
update-mempool "$node/dnn/config/tendermint.toml"
update-mempool "$node/bvnn/config/tendermint.toml"

grep "max-envelopes-per-block" "$node/dnn/config/accumulate.toml" "$node/bvnn/config/accumulate.toml"
echo
grep -A 1 "# Maximum number of transactions in the mempool" "$node/dnn/config/tendermint.toml" "$node/bvnn/config/tendermint.toml"

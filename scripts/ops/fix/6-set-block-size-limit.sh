#!/bin/bash
# image: registry.gitlab.com/accumulatenetwork/accumulate:v1-2-11

set -e

function update-config {
    if `grep -q '^max-envelopes-per-block = ' "$1"`; then
        awk '{gsub(/^max-envelopes-per-block = .*$/, "max-envelopes-per-block = 100");print$0}' "$1" > /tmp/file
    else
        { echo 'max-envelopes-per-block = 100'; cat "$1"; } > /tmp/file
    fi
    cat /tmp/file > "$1"
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

update-config "$node/dnn/config/accumulate.toml"
update-config "$node/bvnn/config/accumulate.toml"

grep -A 1 "max-envelopes-per-block" "$node/dnn/config/accumulate.toml" "$node/bvnn/config/accumulate.toml"

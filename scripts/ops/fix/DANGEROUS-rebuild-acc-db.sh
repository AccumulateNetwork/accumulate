#!/bin/bash
# image: registry.gitlab.com/accumulatenetwork/accumulate:v1-2-9

node="$1"
if [ -z "$node" ]; then
  node=/node
fi
if ! [ -d "$node" ]; then
  >&2 echo "Error: $node is not a directory"
  >&2 echo "Usage: $0 [node dir]"
  exit 1
fi

# Remove the Accumulate database
mv "$node/dnn/data/accumulate.db" "$node/bak-dnn-$(date +%s).db"

# Rollback consensus by one block
cometbft rollback --home "$node/dnn"
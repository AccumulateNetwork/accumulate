#!/bin/bash

node="$1"
if [ -z "$node" ]; then
  node=/node
fi
if ! [ -d "$node" ]; then
  >&2 echo "Error: $node is not a directory"
  >&2 echo "Usage: $0 [node dir]"
  exit 1
fi

# Replace 1679x with 1669x
sed -i -re 's/1679([1-5])/1669\1/g' "$node/bvnn/config/accumulate.toml"
sed -i -re 's/1679([1-5])/1669\1/g' "$node/bvnn/config/tendermint.toml"
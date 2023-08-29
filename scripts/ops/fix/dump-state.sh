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

if ! which debug 2> /dev/null; then
  echo "Downloading debug binary"
  curl -LJ -o debug https://gitlab.com/accumulatenetwork/accman/-/raw/binaries/bin/v1.2.5/debug-linux-amd64
  echo "92a20f6129e9db5fee58f707b5b2fb72690b86f62607fd1576464fec449dcbf8 debug" | sha256sum -c
  chmod +x debug
  mv debug /bin/debug
fi

# Find the `partition-id = "..."` line and extract the BVN name (and make it lower case)
bvn=$(sed -nre 's/^\s+partition-id\s+=\s+"(\w+)"$/\1/p' "$node/bvnn/config/accumulate.toml" | tr '[:upper:]' '[:lower:]')

echo "DN"
debug explore account dn.acme/ledger --node "$node/dnn" 2> /dev/null | jq
echo "Hash: $(debug explore bpt root --node "$node/dnn" 2> /dev/null)"
echo

echo "BVN"
debug explore account bvn-$bvn.acme/ledger --node "$node/bvnn" 2> /dev/null | jq
echo "Hash: $(debug explore bpt root --node "$node/bvnn" 2> /dev/null)"

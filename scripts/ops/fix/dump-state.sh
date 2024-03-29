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

mkdir /tmpbin
export PATH="/tmpbin:$PATH"

echo "Downloading debug binary"
curl -LJ -o debug https://gitlab.com/accumulatenetwork/accumulate/-/raw/files/scripts/bin/v1.2.5/debug-linux-amd64
echo "3838acfe99321b0fbb8c8b148e70785bbf5440d9c9b7a113c680ef7aed5fe50b debug" | sha256sum -c
chmod +x debug
mv debug /tmpbin/debug

# Find the `partition-id = "..."` line and extract the BVN name (and make it lower case)
bvn=$(sed -nre 's/^\s+partition-id\s+=\s+"(\w+)"$/\1/p' "$node/bvnn/config/accumulate.toml" | tr '[:upper:]' '[:lower:]')

echo "DN"
debug explore account dn.acme/ledger --node "$node/dnn" 2> /dev/null | jq
echo "Hash: $(debug explore bpt root --node "$node/dnn" 2> /dev/null)"
echo

echo "BVN"
debug explore account bvn-$bvn.acme/ledger --node "$node/bvnn" 2> /dev/null | jq
echo "Hash: $(debug explore bpt root --node "$node/bvnn" 2> /dev/null)"

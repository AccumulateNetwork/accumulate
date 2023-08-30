#!/bin/bash

set -e

function die {
  >&2 echo "$@"
  exit 1
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

mkdir /tmpbin
export PATH="/tmpbin:$PATH"

function download {
  name=$1
  sha=$2

  echo "Downloading $name binary"
  curl -LJ -o $name https://gitlab.com/accumulatenetwork/accumulate/-/raw/main/scripts/bin/v1.2.5/$name-linux-amd64
  echo "$sha $name" | sha256sum -c
  chmod +x $name
  mv $name /tmpbin/$name
}

download debug 3838acfe99321b0fbb8c8b148e70785bbf5440d9c9b7a113c680ef7aed5fe50b

# Find the `partition-id = "..."` line and extract the BVN name (and make it lower case)
bvn=$(sed -nre 's/^\s+partition-id\s+=\s+"(\w+)"$/\1/p' "$node/bvnn/config/accumulate.toml" | tr '[:upper:]' '[:lower:]')

dnHeight=12614506

case $bvn in
  apollo)
    bvnHeight=16812756
    ;;
  yutu)
    bvnHeight=16085527
    ;;
  chandrayaan)
    bvnHeight=12227202
    ;;
  *)
    die "Error: $bvn is not a known BVN"
    ;;
esac

function check-height {
  dir=$1
  account=$2
  want=$3
  got="$(debug explore account $2 --node "$node/$dir" | jq -re .index)"
  if [ "$got" -ge "$want" ]; then
    die "Error: $dir height does not match: want <=$want, got $got"
  fi
}

# Sanity check on height
check-height dnn dn.acme/ledger $dnHeight
check-height bvnn bvn-$bvn.acme/ledger $bvnHeight

# Delete all databases
rm -rf "$node"/{dnn,bvnn}/data/{*.db,cs.wal}

# Reset the voting state
echo '{ "height": "0", "round": 0, "step": 0 }' > dnn/data/priv_validator_state.json
echo '{ "height": "0", "round": 0, "step": 0 }' > bvnn/data/priv_validator_state.json

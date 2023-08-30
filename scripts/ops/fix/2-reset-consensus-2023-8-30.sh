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
  want=$2
  got="$(cut -d, -f3 <"$node/$dir/config/genesis.json" | sed -nre 's/^\s*"initial_height":\s*"([0-9]+)"/\1/p')"
  if [ "$got" != "$want" ]; then
    die "Error: $dir genesis height does not match: want $want, got $got"
  fi
}

# Sanity check on height
check-height dnn $dnHeight
check-height bvnn $bvnHeight

# Delete all databases
rm -rf "$node"/{dnn,bvnn}/data/{*.db,cs.wal}

# Reset the voting state
echo '{ "height": "0", "round": 0, "step": 0 }' > "$node/dnn/data/priv_validator_state.json"
echo '{ "height": "0", "round": 0, "step": 0 }' > "$node/bvnn/data/priv_validator_state.json"

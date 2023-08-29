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
dnHash=dc623c43ce73defbb99099f8aeea29e2fe7f35d7a4cd87f254a88358b7d8f02f

case $bvn in
  apollo)
    bvnHeight=16812756
    bvnHash=8F16BF3E7608515C47A9CF2115541BEC6162570FB53F007622DD2611314DE9B2
    ;;
  yutu)
    bvnHeight=16085527
    bvnHash=890375bf7677e3ae963fb512738afdacbc473b20e6f04c4b8f2b1c50e58f5531
    ;;
  chandrayaan)
    bvnHeight=12227202
    bvnHash=3D36B58BF3639EF6FFA18BCCF1BD67F430E9BF0DA54226020085C0DE9C70D5D6
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
  if [ "$got" != "$want" ]; then
    die "Error: $dir height does not match: want $want, got $got"
  fi
}

function check-hash {
  dir=$1
  want=$2
  got="$(debug explore bpt root --node "$node/$dir")"
  if [ "$got" != "$want" ]; then
    die "Error: $dir hash does not match: want $want, got $got"
  fi
}

check-height dnn dn.acme/ledger $dnHeight
check-height bvnn bvn-$bvn.acme/ledger $bvnHeight
check-hash dnn $dnHash
check-hash bvnn $bvnHash

accumulated reset consensus "$node/dnn"
accumulated reset consensus "$node/bvnn"

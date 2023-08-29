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

function download {
  name=$1
  sha=$2

  if which $name 2> /dev/null; then
    return
  fi

  echo "Downloading $name binary"
  curl -LJ -o $name https://gitlab.com/accumulatenetwork/accumulate/-/raw/main/scripts/bin/v1.2.5/$name-linux-amd64
  echo "$sha $name" | sha256sum -c
  chmod +x $name
  mv $name /bin/$name
}

download debug 3838acfe99321b0fbb8c8b148e70785bbf5440d9c9b7a113c680ef7aed5fe50b
download accumulated a021da83ba133f3bdbc646c9a4d3f6a25083d6f055e1f0b4025c31b44833b2e3

# Find the `partition-id = "..."` line and extract the BVN name (and make it lower case)
bvn=$(sed -nre 's/^\s+partition-id\s+=\s+"(\w+)"$/\1/p' "$node/bvnn/config/accumulate.toml" | tr '[:upper:]' '[:lower:]')

dnHeight=12614506
dnHash=dc623c43ce73defbb99099f8aeea29e2fe7f35d7a4cd87f254a88358b7d8f02f

case $bvn in
  apollo)
    bvnHeight=16812756
    bvnHash=2a4798da31ca73478ecef3b60a519d58f51950e9ea0bf2c73180dba8b7554703
    ;;
  yutu)
    bvnHeight=16085527
    bvnHash=890375bf7677e3ae963fb512738afdacbc473b20e6f04c4b8f2b1c50e58f5531
    ;;
  chandrayaan)
    bvnHeight=12227202
    bvnHash=0b4e7360977c03656103ce0bdf511cb66b016f8ceb3c16f10921b9520fb26ec7
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

echo accumulated reset consensus "$node/dnn"
echo accumulated reset consensus "$node/bvnn"

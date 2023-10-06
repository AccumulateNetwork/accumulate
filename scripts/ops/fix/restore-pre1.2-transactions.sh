#!/bin/bash
# image: registry.gitlab.com/accumulatenetwork/accumulate:v1-2-9

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

mkdir /tmpbin
export PATH="/tmpbin:$PATH"

function download-bin {
  name=$1
  sha=$2

  download bin/v1.2.9/$name-linux-amd64 $name $sha
  chmod +x $name
  mv $name /tmpbin/$name
}

function download {
  path=$1
  as=$2
  sha=$3

  echo -n "Downloading $path... "
  if [ -f $as ] && (echo "$sha $as" | sha256sum -c); then
    echo "already have the correct file"
    return
  fi

  curl -LJ -o $as https://gitlab.com/accumulatenetwork/accumulate/-/raw/files/scripts/$path
  echo "$sha $as" | sha256sum -c
  echo "done"
}

download-bin dbrepair 08465f3150b6c9906c7996fb7d3e2f8cbbb8add280f3ad788526147fe767b8b0
download data/pre-v2-txn-dn-fix.dat dn.fix 9c83fb66cd42fdc7970a017978c53d031e1045f340e04a9e77188c3dbda8e63e

case $bvn in
  apollo)
    download data/pre-v2-txn-$bvn-fix.dat bvn.fix 9f67fc53db86322ebb958ef3fe404db6ff335a1e4484a782ab5cc6068baf6f2d
    ;;
  yutu)
    download data/pre-v2-txn-$bvn-fix.dat bvn.fix 4c21a63dc6feca3b48b9f1a5e38b34024ce32c10c24fff6fb893d1e25c671ee9
    ;;
  chandrayaan)
    download data/pre-v2-txn-$bvn-fix.dat bvn.fix 812b7c0f7460bff2cec112ba01b1903462e6b5d8ddcb81ca38863544ee2b3af1
    ;;
  *)
    die "Error: $bvn is not a known BVN"
    ;;
esac

dbrepair applyMissing dn.fix "$node/dnn/data/accumulate.db"
dbrepair applyMissing bvn.fix "$node/bvnn/data/accumulate.db"
rm dn.fix bvn.fix

echo
echo
echo "Success!"
echo

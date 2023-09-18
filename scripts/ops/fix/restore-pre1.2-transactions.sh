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

  echo "Downloading $path"
  curl -LJ -o $as https://gitlab.com/accumulatenetwork/accumulate/-/raw/main/scripts/$path
  echo "$sha $as" | sha256sum -c
}

download-bin dbrepair 66df562c69aa0fab21c36dd0c1a9ec24b1de91fe2e81eec6051489d29ecc4e8b
download data/pre-v2-txn-dn-fix.dat dn.fix 9c83fb66cd42fdc7970a017978c53d031e1045f340e04a9e77188c3dbda8e63e

case $bvn in
  apollo)
    download data/pre-v2-txn-$bvn-fix.dat bvn.fix 9f67fc53db86322ebb958ef3fe404db6ff335a1e4484a782ab5cc6068baf6f2d
    ;;
  yutu)
    download data/pre-v2-txn-$bvn-fix.dat bvn.fix 97904b2dc7d344841b2b14f95a7a890a61bcc5869a028c30ed71a36a2091d9bb
    ;;
  chandrayaan)
    download data/pre-v2-txn-$bvn-fix.dat bvn.fix 90db00407b67be369a1b060fb336017215b852bbb1edbea273be225df359433f
    ;;
  *)
    die "Error: $bvn is not a known BVN"
    ;;
esac

dbrepair applyFix dn.fix "$node/dnn/data/accumulate.db"
dbrepair applyFix bvn.fix "$node/bvnn/data/accumulate.db"

echo
echo
echo "Success!"
echo

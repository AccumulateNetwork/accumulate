#!/bin/bash
# image: registry.gitlab.com/accumulatenetwork/accumulate:v1-2-13

node="$1"
if [ -z "$node" ]; then
  node=/node
fi
if ! [ -d "$node" ]; then
  >&2 echo "Error: $node is not a directory"
  >&2 echo "Usage: $0 [node dir]"
  exit 1
fi

  node=/node
>&2 printf '\033[33;1mThis will run forever in a loop. Ctrl+C at any time to stop. [Enter] to continue.\033[0m\n'
read -s

while true; do
    >&2 printf '\033[33;1mFlatten DNN\033[0m\n'
    debug badger flatten "$node/dnn/data/accumulate.db"
    >&2 printf '\033[33;1mFlatten BVNN\033[0m\n'
    debug badger flatten "$node/bvnn/data/accumulate.db"
    >&2 printf '\033[33;1mCompact DNN\033[0m\n'
    debug badger compact "$node/dnn/data/accumulate.db"
    >&2 printf '\033[33;1mCompact BVNN\033[0m\n'
    debug badger compact "$node/bvnn/data/accumulate.db"
done

#!/bin/bash
# image: registry.gitlab.com/accumulatenetwork/accumulate:v1-2-11

node="$1"
if [ -z "$node" ]; then
  node=/node
fi
if ! [ -d "$node" ]; then
  >&2 echo "Error: $node is not a directory"
  >&2 echo "Usage: $0 [node dir]"
  exit 1
fi

>&2 echo 'This will `rm -rf "'"$node"'/{dnn,bvnn}/data/{*db,cs.wal}"`. Are you sure you want to do that? Ctrl-C to cancel or [Enter] to continue.'
read -s

# Nuke all the state
for i in {3..1}; do
    >&2 echo "$i"
    sleep 1
done
>&2 echo "Resetting state"
rm -rf "$node"/{dnn,bvnn}/data/{*.db,cs.wal}

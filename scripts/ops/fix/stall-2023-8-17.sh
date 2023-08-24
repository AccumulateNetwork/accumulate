#!/bin/bash

set -e

SCRIPTS=$( cd "$( dirname "${BASH_SOURCE[0]}" )/../.." && pwd )

NODE="$1"
if [ -z "$NODE" ]; then
    >&2 echo "Usage: $0 [node dir]"
    exit 1
fi

dbrepair applyFix "${SCRIPTS}/data/dn-12614506.fix" "$NODE/dnn/data/accumulate.db"
tendermint rollback --home "$NODE/dnn"; echo
tendermint set-app-version 1 --home "$NODE/dnn"

#!/bin/bash

set -e

if [ -z "$1" ]; then
    >&2 echo "Usage: $0 [node dir]"
    exit 1
fi

# TODO Fixup Accumulate

tendermint rollback --home $1/dnn; echo
tendermint set-app-version 1 --home $1/dnn

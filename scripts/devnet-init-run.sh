#!/bin/bash

go run ./cmd/accumulated \
    init devnet \
    -w .nodes \
    --reset \
    --faucet-seed ci \
    --globals '{"executorVersion": "v2", "oracle": { "price": 50000000 } }' \
    "$@"

go run ./cmd/accumulated \
    run devnet \
    -w .nodes \
    --faucet-seed ci
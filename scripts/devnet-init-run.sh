#!/bin/bash

# Modern DevNet setup using the accumulated daemon
# The old 'init devnet' command no longer exists
# Use 'run devnet --init-only' instead

echo "Initializing DevNet..."
go run ./cmd/accumulated run devnet \
    --init-only \
    --reset \
    -w .nodes \
    "$@"

echo "Starting DevNet..."
go run ./cmd/accumulated run devnet \
    -w .nodes
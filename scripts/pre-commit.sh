#!/bin/bash

set -e

if [ -n "$NOMOCK" ]; then
    SKIP="-skip=.*mockery.*"
fi

go mod tidy
go generate -x $SKIP ./...
go run github.com/rinchsan/gosimports/cmd/gosimports -w .
go run ./tools/cmd/golangci-lint run --verbose --timeout=10m

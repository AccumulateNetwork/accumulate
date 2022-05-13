#!/bin/bash

go mod tidy
go generate -x ./...
go run github.com/rinchsan/gosimports/cmd/gosimports -w .
go vet ./...
go run ./tools/cmd/golangci-lint run --verbose --timeout=10m

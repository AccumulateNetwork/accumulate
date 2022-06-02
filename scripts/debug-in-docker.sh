#!/bin/bash

apk add --no-cache go
go install github.com/go-delve/delve/cmd/dlv@latest

while true; do
    ~/go/bin/dlv dap --listen=:40000
done
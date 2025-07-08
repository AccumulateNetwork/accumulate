#!/bin/bash

# Build accumulated binary without BPT commands
cd /home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate
go build -o accumulated ./cmd/accumulated/main.go \
  ./cmd/accumulated/cmd_*.go \
  ./cmd/accumulated/run/*.go \
  ./cmd/accumulated/run/cmd_*.go \
  !./cmd/accumulated/cmd_add_bpt_section.go \
  !./cmd/accumulated/cmd_debug_bpt.go

# Check if build was successful
if [ $? -eq 0 ]; then
  echo "✅ Build successful: accumulated binary created without BPT commands"
  echo "Binary location: $(pwd)/accumulated"
else
  echo "❌ Build failed"
  exit 1
fi

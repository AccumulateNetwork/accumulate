#!/bin/bash

# Script to disable BPT commands in accumulated to fix build errors
# This allows building the binary without fixing the BPT-related API incompatibilities

cd /home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate

# Comment out the BPT command imports and registrations in cmd_sync.go
sed -i 's/cmdDebugBpt,/\/\/ cmdDebugBpt,/g' cmd/accumulated/cmd_sync.go
sed -i 's/cmdAddBptSection,/\/\/ cmdAddBptSection,/g' cmd/accumulated/cmd_sync.go

# Rename the BPT command files to prevent them from being compiled
if [ -f cmd/accumulated/cmd_add_bpt_section.go ]; then
  mv cmd/accumulated/cmd_add_bpt_section.go cmd/accumulated/cmd_add_bpt_section.go.disabled
fi

if [ -f cmd/accumulated/cmd_debug_bpt.go ]; then
  mv cmd/accumulated/cmd_debug_bpt.go cmd/accumulated/cmd_debug_bpt.go.disabled
fi

echo "BPT commands disabled. Now you can build the accumulated binary."
echo "Run: go build -o accumulated ./cmd/accumulated"

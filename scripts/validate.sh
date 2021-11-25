#!/bin/bash

# Stop immediately on error
set -e

# Setup
go install ./cmd/cli
export PATH="${PATH}:$(go env GOPATH)/bin"
cli key import mnemonic ${MNEMONIC}

# Generate Lite account + faucet
cli account generate
LITE=$(cli account list)
cli faucet ${LITE}
sleep 3

# Add credits
cli credits ${LITE} ${LITE} 100
sleep 3

# Generate keys
cli key generate keytest-0-0
cli key generate keytest-1-0
cli key generate keytest-1-1
cli key generate keytest-2-0
cli key generate keytest-2-1

# Create ADI
cli adi create ${LITE} keytest keytest-0-0 book page0
sleep 3

# Add pages to the book
cli page create keytest/book keytest-0-0 keytest/page1 keytest-1-0
cli page create keytest/book keytest-0-0 keytest/page2 keytest-2-0
sleep 3

# Add a key to page1 using a page1 key
cli page key add keytest/page1 keytest-1-0 1 1 keytest-1-1

# Add a key to page2 using a page1 key
cli page key add keytest/page2 keytest-1-0 1 1 keytest-2-1

# Create ADI Token Account
cli account create keytest keytest-0-0 0 1 keytest/tokens ACME keytest/book
sleep 3

# Send tokens
cli tx create ${LITE} keytest/tokens 5
sleep 3

# Add credits
cli credits keytest/tokens keytest-0-0 0 1 keytest/page0 125
sleep 3
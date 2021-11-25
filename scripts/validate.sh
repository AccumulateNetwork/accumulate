#!/bin/bash

set -e # Stop immediately on error

function section {
    echo -e '\033[1m'"$1"'\033[0m'
}

function ensure-key {
    if ! cli key list 2>&1 | grep "$1"; then
        cli key generate "$1"
    fi
}

section "Setup"
go install ./cmd/cli
export PATH="${PATH}:$(go env GOPATH)/bin"
[ -z "${MNEMONIC}" ] || cli key import mnemonic ${MNEMONIC}
echo

section "Generate a Lite Token Account"
cli account list &> /dev/null || cli account generate
LITE=$(cli account list 2>&1 | head -1)
cli faucet ${LITE}
sleep 3

section "Add credits to lite account"
cli credits ${LITE} ${LITE} 100
sleep 3

section "Generate keys"
ensure-key keytest-0-0
ensure-key keytest-1-0
ensure-key keytest-1-1
ensure-key keytest-2-0
ensure-key keytest-2-1
echo

section "Create an ADI"
cli adi create ${LITE} keytest keytest-0-0 book page0
sleep 3

section "Create additional Key Pages"
cli page create keytest/book keytest-0-0 keytest/page1 keytest-1-0
cli page create keytest/book keytest-0-0 keytest/page2 keytest-2-0
sleep 3

# section "Add a key to page 1 using a key from page 1"
# cli page key add keytest/page1 keytest-1-0 1 1 keytest-1-1

# section "Add a key to page 2 using a key from page 1"
# cli page key add keytest/page2 keytest-1-0 1 1 keytest-2-1

section "Create an ADI Token Account"
cli account create keytest keytest-0-0 0 1 keytest/tokens ACME keytest/book
sleep 3

section "Send tokens from the lite token account to the ADI token account"
cli tx create ${LITE} keytest/tokens 5
sleep 3

section "Add credits to the ADI token account"
cli credits keytest/tokens keytest-0-0 0 1 keytest/page0 125
sleep 3
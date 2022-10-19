#!/bin/bash

set -e
cd $( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-sdk --package wallet --out wallet_api_sdk_gen.go ../../cmd/accumulate/walletd/api/methods.yml
sed -i 's|gitlab.com/accumulatenetwork/accumulate/internal/api/v2|gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate/walletd/api|g' wallet_api_sdk_gen.go
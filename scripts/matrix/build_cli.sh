#!/bin/bash

# Ensure Go's bin folder is on the path
if ! echo $PATH | sed 's/:/\n/g' | grep -q '^'$(go env GOPATH)'/bin$' ; then
    export PATH="${PATH}:$(go env GOPATH)/bin"
fi

# Check if the CLI is installed
if ! which cli &> /dev/null ; then
	echo "Installing the CLI to $(go env GOPATH)/bin"
    (cd $(git rev-parse --show-toplevel) && go install ./cmd/cli) || exit 1

	if ! which cli &> /dev/null ; then
        echo "Failed to install the CLI"
		exit 1
	fi
fi

export cli=cli
#!/bin/bash

REF=$1
if [ -z "$REF" ] ; then
    echo "Usage: $0 <git-ref>"
    exit 1
fi

if [ -e ~/.local/bin/accumulated ]; then
    mv ~/.local/bin/accumulated{,-old}
fi

API_V4=https://gitlab.com/api/v4
PROJECT=29762666
mkdir -p ~/.local/bin
curl -LJ -o ~/.local/bin/accumulated "${API_V4}/projects/${PROJECT}/jobs/artifacts/${REF}/raw/accumulated-linux-amd64?job=build"
chmod +x ~/.local/bin/accumulated
#!/bin/bash

if [ -z "$ACC_API" ] ; then
    if [ -z $1 ]; then
        echo "You must pass IP:Port for the server to use or set ACC_API"
        exit 1
    fi
    export ACC_API="$1"
fi
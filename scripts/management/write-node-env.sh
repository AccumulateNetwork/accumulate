#!/bin/bash

IP=$1
BVC=$2
NODE=$3
ARCH=$4

if [ -z "$IP" ] || [ -z "$BVC" ] || [ -z "$NODE" ] || [ -z "$ARCH" ] ; then
    echo "Usage: $0 <ip> <bvc> <node> amd64|arm64"
    exit 1
fi

case "$ARCH" in
    arm64|amd64) ;; # OK
    *)
        echo "Usage: $0 <ip> <bvc> <node> amd64|arm64"
        exit 1
        ;;
esac

ssh ec2-user@${IP} 'echo "BVC='${BVC}$'\n''NODE='${NODE}$'\n''ARCH='${ARCH}'" > ~/node.env'
#!/bin/bash

IP=$1
BVC=$2
NODE=$3

if [ -z "$IP" ] || [ -z "$BVC" ] || [ -z "$NODE" ] ; then
    echo "Usage: $0 <ip> <bvc> <node>"
    exit 1
fi

ssh ec2-user@${IP} 'echo "BVC='${BVC}$'\n''NODE='${NODE}'" > ~/node.env'
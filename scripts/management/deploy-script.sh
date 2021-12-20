#!/bin/bash

for ip in "${@:3}"; do
    scp "$1"  "$2" ec2-user@${ip}:
    ssh ec2-user@${ip} "chmod +x '$1' && chmod +x '$2'"
done
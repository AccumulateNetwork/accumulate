#!/bin/bash

for ip in "${@:2}"; do
    scp "$1" ec2-user@${ip}:
    ssh ec2-user@${ip} "chmod +x '$1'"
done
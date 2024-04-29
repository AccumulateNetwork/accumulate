#!/bin/bash

# Launches delve within a container and attaches it to PID 1
#
# Usage: ./scripts/delve-in-docker.sh <container>

SCRIPT="dlv attach 1 --headless --listen=:2345 --accept-multiclient --api-version=2"

docker exec --privileged -it "$1" bash -c "$SCRIPT"
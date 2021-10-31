#!/bin/bash

API_V4=https://gitlab.com/api/v4
PROJECT=29762666
mkdir -p ~/.local/bin
curl -LJ -o ~/.local/bin/accumulated "${API_V4}/projects/${PROJECT}/jobs/artifacts/test-dev/raw/accumulated-linux-amd64?job=build"
chmod +x ~/.local/bin/accumulated
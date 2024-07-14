#!/bin/bash

# CD to the directory containing this file
cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null

# Make sure dependencies are installed
npm ci &> /dev/null

# Run the script
node main.js "$@"
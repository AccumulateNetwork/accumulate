#!/bin/bash

# Custom hexdump script with better formatting
# Usage: ./hexdump.sh <file> <start_offset> <length>

FILE=$1
START=$2
LENGTH=$3

# Function to print formatted hex
hexdump -n $LENGTH -s $START -e '"%08.8_ax  " 8/1 "%02x " "  " 8/1 "%02x " "  " 8/1 "%02x " "  " 8/1 "%02x " "  |" 32/1 "%_p" "|\n"' $FILE

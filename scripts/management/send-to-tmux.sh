#!/bin/bash

for i in `seq 0 $(($1-1))`; do
    tmux send-keys -t $i "${@:2}"
done
#!/bin/bash

tmux send-keys -t 0 "$@"
tmux send-keys -t 1 "$@"
tmux send-keys -t 2 "$@"
tmux send-keys -t 3 "$@"
tmux send-keys -t 4 "$@"
tmux send-keys -t 5 "$@"

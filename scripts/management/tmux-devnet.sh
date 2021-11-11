#!/bin/bash

tmux new-session -d -s acc
tmux set pane-border-status top
tmux rename-window 'Accumulate'
tmux split-window -h -t 0
tmux select-layout even-horizontal
tmux split-window -v -t 0
tmux split-window -v -t 2
tmux send-keys -t 0 "printf '\\033]2;BVC 0, Node 0 (3.21.112.155)\\033\\\\'" 'C-m' 'ssh ec2-user@3.21.112.155' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 1 "printf '\\033]2;BVC 0, Node 1 (18.221.144.247)\\033\\\\'" 'C-m' 'ssh ec2-user@18.221.144.247' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 2 "printf '\\033]2;BVC 1, Node 0 (3.142.136.223)\\033\\\\'" 'C-m' 'ssh ec2-user@3.142.136.223' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 3 "printf '\\033]2;BVC 1, Node 1 (18.217.135.168)\\033\\\\'" 'C-m' 'ssh ec2-user@18.217.135.168' 'C-m' 'screen -r' 'C-m'
bash -c 'sleep 0.1; tmux split-window -f -l 2' &
tmux -2 attach-session -t acc
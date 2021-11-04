#!/bin/bash

tmux new-session -d -s acc
tmux set pane-border-status top
tmux rename-window 'Accumulate'
tmux split-window -h -t 0
tmux split-window -h -t 0
tmux select-layout even-horizontal
tmux split-window -v -t 0
tmux split-window -v -t 2
tmux split-window -v -t 4
tmux send-keys -t 0 "printf '\\033]2;BVC 0, Node 0 (3.140.120.192)\\033\\\\'" 'C-m' 'ssh ec2-user@3.140.120.192' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 1 "printf '\\033]2;BVC 0, Node 1 (18.220.147.250)\\033\\\\'" 'C-m' 'ssh ec2-user@18.220.147.250' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 2 "printf '\\033]2;BVC 1, Node 0 (65.0.156.146)\\033\\\\'" 'C-m' 'ssh ec2-user@65.0.156.146' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 3 "printf '\\033]2;BVC 1, Node 1 (13.234.254.178)\\033\\\\'" 'C-m' 'ssh ec2-user@13.234.254.178' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 4 "printf '\\033]2;BVC 2, Node 0 (13.48.159.117)\\033\\\\'" 'C-m' 'ssh ec2-user@13.48.159.117' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 5 "printf '\\033]2;BVC 2, Node 1 (16.170.126.251)\\033\\\\'" 'C-m' 'ssh ec2-user@16.170.126.251' 'C-m' 'screen -r' 'C-m'
bash -c 'sleep 0.1; tmux split-window -f -l 2' &
tmux -2 attach-session -t acc
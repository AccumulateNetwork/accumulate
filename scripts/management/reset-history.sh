#!/bin/bash

DATE=$(date +%Y-%m-%d-%H-%M-%S)

tmux send-keys -t 0 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'
tmux send-keys -t 1 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'
tmux send-keys -t 2 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'
tmux send-keys -t 3 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'
tmux send-keys -t 4 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'
tmux send-keys -t 5 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'
tmux send-keys -t 6 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'
tmux send-keys -t 7 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'
tmux send-keys -t 8 'source node.env' 'C-m' 'mkdir -p archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}' 'C-m' 'mv ~/.accumulate/bvc${BVC}/Node${NODE}/{data,valacc.db} archive/'"${DATE}"'/bvc-${BVC}-node-${NODE}/' 'C-m'

tmux send-keys -t 0 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'
tmux send-keys -t 1 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'
tmux send-keys -t 2 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'
tmux send-keys -t 3 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'
tmux send-keys -t 4 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'
tmux send-keys -t 5 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'
tmux send-keys -t 6 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'
tmux send-keys -t 7 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'
tmux send-keys -t 8 'mkdir ~/.accumulate/bvc${BVC}/Node${NODE}/data' 'C-m' "echo '{\"height\": \"0\",\"round\": 0,\"step\": 0}'"' > ~/.accumulate/bvc${BVC}/Node${NODE}/data/priv_validator_state.json' 'C-m'

tmux send-keys -t 0 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 1 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 2 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 3 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 4 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 5 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 6 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 7 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'
tmux send-keys -t 8 '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'

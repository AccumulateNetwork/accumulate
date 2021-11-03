# Deploying

## Screen Help

- Detach from the current session with `[Ctrl-A]`, `[Ctrl-D]`

## TestNet 1.0 (Factom AWS)

### Scripts

- Client-side
  - `./deploy-script.sh ${FILE} ${LIST_OF_IPS}` - SCPs FILE to each server
  - `./tmux-nodes.sh` - Opens up a tmux session with six panes, starts an SSH
    session for each server, and reconnects to the Accumulate screen session
  - `./send-to-tmux.sh ${COMMANDS}` - Sends COMMANDS to each of the six tmux
    panes
  - `./write-node-env.sh ${IP} ${BVC} ${NODE}` - Uses SSH to connect to IP and
    write the BVC and node numbers to `~/node.env`
- Server-side
  - `./download-accumulate.sh ${REF}` - Renames the `accumulated` binary to
    `accumulated-old` (if it exists) and downloads the latest AMD64 build from
    the latest CI artifacts of REF to `~/.local/bin/accumulated`.
  - `./launch-node.sh` - Reads `~/node.env` and launches `accumulated` in a
    screen session with the correct config.
  - `./reset-acc-db.sh` - **Do not use when the node is running.** Deletes the
    accumulated key-value store. When the node is booted, Tendermint will replay
    all the transaction history, rebuilding the key-value store.
  - `./reset-history.sh` - **Resets all history and state data for the node.**
    Do not use when the node is running. Moves everything to date-stamped
    archive folder (instead of deleting it).

### Deploying a new build

`LIST_OF_IPS` is a space-separated list of the external IP addresses for the EC2
nodes. `RELEASE_REF` is the branch or tag that is going to be deployed.

1. Open up an ssh session for each server and attach to the screen session
   1. `ssh ec2-user@${IP}`
   2. `screen -r`
2. From another terminal, deploy the new build
   1. `cd scripts/management`
   2. `for IP in ${LIST_OF_IPS}; do ssh ec2-user@${IP} 'mv ~/.local/bin/accumulated{,-old} && ./download-accumulate.sh ${RELEASE_REF}'; done`
3. **One at a time**, for each screen session, relaunch with the new build
   1. Terminate the node with `[Ctrl-C]`
   2. Relaunch the node with `./launch-node.sh`
   3. Optionally, attach to the new screen session with `screen -r`
# Deploying

## Screen Help

- Detach from the current session with `[Ctrl-A]`, `[Ctrl-D]`

## TestNet 0.2 (Factom AWS)

1. Open up an ssh session for each server and attach to the screen session
   1. `ssh ec2-user@${IP}`
   2. `screen -r`
2. From another terminal, deploy the new build
   1. `cd scripts/management`
   2. `for IP in ${LIST_OF_IPS}; do ssh ec2-user@${IP} 'mv ~/.local/bin/accumulated{,-old} && ./download-accumulate.sh'; done`
3. **One at a time**, for each screen session, relaunch with the new build
   1. Terminate the node with `[Ctrl-C]`
   2. Relaunch the node with `./launch-node.sh`
   3. Optionally, attach to the new screen session with `screen -r`
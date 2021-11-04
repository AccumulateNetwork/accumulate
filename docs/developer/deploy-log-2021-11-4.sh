# CD to the root of the repo

# Copy config from BVC0 Node0 (which already has the config for everything
# because I was lazy)
scp -r ec2-user@3.140.120.192:.accumulate all

go run ./cmd/accumulated init -n BVC0 -w all/bvc0 -r BVC0,BVC1,BVC2
go run ./cmd/accumulated init -n BVC1 -w all/bvc1 -r BVC0,BVC1,BVC2
go run ./cmd/accumulated init -n BVC2 -w all/bvc2 -r BVC0,BVC1,BVC2

# MAKE SURE TO REVERT CHANGES TO THE GENESIS TIME in config/genesis.json
# Also, copy the genesis time from the existing nodes to the new node

# Copy the config for all the node2's (but not everything else)
ssh ec2-user@52.89.160.158 'mkdir -p ~/.accumulate/bvc0'
ssh ec2-user@44.229.57.187 'mkdir -p ~/.accumulate/bvc1'
ssh ec2-user@34.214.215.210 'mkdir -p ~/.accumulate/bvc2'
scp -r all/bvc0/Node2 ec2-user@52.89.160.158:.accumulate/bvc0/
scp -r all/bvc1/Node2 ec2-user@44.229.57.187:.accumulate/bvc1/
scp -r all/bvc2/Node2 ec2-user@34.214.215.210:.accumulate/bvc2/

# Copy ONLY config.toml and genesis.json to the existing nodes
function update-config {
    IP="$1"
    BVC="$2"
    NODE="$3"
    scp all/bvc${BVC}/Node${NODE}/config/{config.toml,genesis.json} ec2-user@${IP}:.accumulate/bvc${BVC}/Node${NODE}/config
}
update-config 3.140.120.192 0 0
update-config 18.220.147.250 0 1
update-config 65.0.156.146 1 0
update-config 13.234.254.178 1 1
update-config 13.48.159.117 2 0
update-config 16.170.126.251 2 1

#
update-config 52.89.160.158 0 2
update-config 44.229.57.187 1 2
update-config 34.214.215.210 2 2

cd scripts/management

# Update node.env for amd64 servers and redeploy download script
./write-node-env.sh 3.140.120.192 0 0 amd64
./write-node-env.sh 18.220.147.250 0 1 amd64
./write-node-env.sh 65.0.156.146 1 0 amd64
./write-node-env.sh 13.234.254.178 1 1 amd64
./write-node-env.sh 13.48.159.117 2 0 amd64
./write-node-env.sh 16.170.126.251 2 1 amd64
./deploy-script.sh download-accumulate.sh 3.140.120.192 18.220.147.250 65.0.156.146 13.234.254.178 13.48.159.117 16.170.126.251

# Write node.env for the new nodes and deploy scripts
./write-node-env.sh 52.89.160.158 0 2 arm64
./write-node-env.sh 44.229.57.187 1 2 arm64
./write-node-env.sh 34.214.215.210 2 2 arm64
./deploy-script.sh download-accumulate.sh 52.89.160.158 44.229.57.187 34.214.215.210
./deploy-script.sh launch-node.sh 52.89.160.158 44.229.57.187 34.214.215.210
./deploy-script.sh reset-acc-db.sh 52.89.160.158 44.229.57.187 34.214.215.210

# Make the binary directory
ssh ec2-user@52.89.160.158 'mkdir -p ~/.local/bin'
ssh ec2-user@44.229.57.187 'mkdir -p ~/.local/bin'
ssh ec2-user@34.214.215.210 'mkdir -p ~/.local/bin'

# Deploy the latest binary from release-v0.2 to all the nodes
./send-to-tmux.sh 'source ~/node.env' 'C-m' '~/.accumulate/bvc${BVC}/Node${NODE}' 'C-m'
./send-to-tmux.sh 'mv ~/.local/bin/accumulated{,-old}' 'C-m' './download-accumulate.sh release-v0.2' 'C-m'
./send-to-tmux.sh '~/launch-node.sh' 'C-m' 'screen -r' 'C-m'

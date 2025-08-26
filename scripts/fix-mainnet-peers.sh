#!/bin/bash

# Fix MainNet Peer Databases
# This script clears stale peer information from mainnet nodes

echo "======================================"
echo "Accumulate MainNet Peer Database Fix"
echo "======================================"
echo ""

# Node information
declare -A NODES
NODES["apollo"]="i-012ecfdd22018769d|23.22.212.106|us-east-1"
NODES["yutu"]="i-0f664d46c69ff7455|54.234.31.209|us-east-1"
NODES["chandrayaan"]="i-0fba089d0418cf699|54.85.31.44|us-east-1"

# Generate SSH key
echo "Generating temporary SSH key..."
rm -f /tmp/mainnet_fix_key*
ssh-keygen -t rsa -b 2048 -f /tmp/mainnet_fix_key -N '' -q

for node_name in apollo yutu chandrayaan; do
    IFS='|' read -r instance_id ip region <<< "${NODES[$node_name]}"
    
    echo ""
    echo "Processing $node_name ($ip)..."
    echo "--------------------------------"
    
    # Send SSH key
    echo "  Sending SSH key..."
    aws ec2-instance-connect send-ssh-public-key \
        --region $region \
        --instance-id $instance_id \
        --instance-os-user ubuntu \
        --ssh-public-key file:///tmp/mainnet_fix_key.pub \
        --output json > /dev/null 2>&1
    
    if [ $? -ne 0 ]; then
        echo "  ERROR: Failed to send SSH key to $node_name"
        continue
    fi
    
    # Connect and fix
    echo "  Connecting to $node_name..."
    ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 \
        -i /tmp/mainnet_fix_key ubuntu@$ip << 'ENDSSH' 2>/dev/null
        
        echo "  Finding accumulated process..."
        PID=$(pgrep accumulated)
        if [ -z "$PID" ]; then
            echo "  WARNING: accumulated not running"
        else
            echo "  Found accumulated PID: $PID"
            
            # Try to find where it's running from
            if [ -d "/proc/$PID" ]; then
                CWD=$(sudo readlink /proc/$PID/cwd)
                echo "  Working directory: $CWD"
            fi
            
            # Common locations for peer databases
            PEER_LOCATIONS=(
                "/var/lib/accumulate/*/peers.json"
                "/node/*/peers.json"
                "/home/*/accumulate/*/peers.json"
                "/root/.accumulate/*/peers.json"
                "/data/*/peers.json"
                "$CWD/*/peers.json"
            )
            
            echo "  Searching for peer databases..."
            for location in "${PEER_LOCATIONS[@]}"; do
                for file in $location; do
                    if [ -f "$file" ]; then
                        echo "  Found: $file"
                        echo "  Backing up to ${file}.backup"
                        sudo cp "$file" "${file}.backup"
                        echo "  Clearing peer database"
                        echo '{"peers":[]}' | sudo tee "$file" > /dev/null
                    fi
                done
            done
            
            # Also look for BadgerDB peer stores
            echo "  Searching for BadgerDB peer stores..."
            sudo find / -type d -name "peerdb" -o -name "peer-db" 2>/dev/null | while read dir; do
                echo "  Found peer DB directory: $dir"
                echo "  Moving to ${dir}.backup"
                sudo mv "$dir" "${dir}.backup"
            done
            
            # Restart accumulated
            echo "  Attempting to restart accumulated..."
            
            # Try systemd first
            if sudo systemctl restart accumulated 2>/dev/null; then
                echo "  Restarted via systemd"
            # Try docker
            elif sudo docker restart $(sudo docker ps -q --filter ancestor=accumulated) 2>/dev/null; then
                echo "  Restarted via docker"
            # Try direct kill and hope it auto-restarts
            elif sudo kill -HUP $PID 2>/dev/null; then
                echo "  Sent HUP signal to accumulated"
            else
                echo "  WARNING: Could not restart accumulated automatically"
                echo "  Manual restart may be required"
            fi
        fi
ENDSSH
    
    echo "  Done with $node_name"
done

echo ""
echo "======================================"
echo "Peer Database Fix Complete"
echo "======================================"
echo ""
echo "Next steps:"
echo "1. Wait 30 seconds for nodes to stabilize"
echo "2. Test with: ./debug test-p2p mainnet"
echo "3. Test with: ./debug sequence mainnet"
echo ""
echo "If issues persist:"
echo "- Check node logs for errors"
echo "- Verify nodes can reach each other on port 16593"
echo "- Ensure bootstrap server is running correctly"
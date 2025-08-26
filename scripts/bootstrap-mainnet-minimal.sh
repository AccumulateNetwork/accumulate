#!/bin/bash
# Minimal MainNet Bootstrap Configuration
# This script configures the bootstrap server to only advertise active nodes
# Last Updated: August 2024

echo "======================================"
echo "Accumulate MainNet Bootstrap Update"
echo "======================================"
echo ""
echo "Current active nodes:"
echo "  - apollo-mainnet (23.22.212.106)"
echo "  - yutu-mainnet (54.234.31.209)"
echo "  - chandrayaan-mainnet (54.85.31.44)"
echo ""

# Stop existing bootstrap
echo "Stopping existing bootstrap container..."
docker stop accumulate-bootstrap 2>/dev/null
docker rm accumulate-bootstrap 2>/dev/null

# Start with minimal peer set
echo "Starting bootstrap with minimal peer set..."
docker run -d \
    --restart unless-stopped \
    -v accumulate-bootstrap:/data \
    --name accumulate-bootstrap \
    -p 0.0.0.0:16593:16593 \
    -p 0.0.0.0:16693:16693 \
    --entrypoint accumulated-bootstrap \
    registry.gitlab.com/accumulatenetwork/accumulate:seed \
    --key /data/key \
    --peer /dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn \
    --peer /dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWJqp6jpagL2cJwhBX3aWJvqCUf46ceYxpqQXKFrgPGRCT \
    --listen /ip4/0.0.0.0/tcp/16593 \
    --listen /ip4/0.0.0.0/udp/16593/quic \
    --listen /ip4/0.0.0.0/tcp/16693 \
    --listen /ip4/0.0.0.0/udp/16693/quic \
    --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
    --external /dns/bootstrap.accumulate.defidevs.io/udp/16593/quic \
    --log-level info

echo ""
echo "Bootstrap server updated successfully!"
echo ""
echo "TODO: Determine chandrayaan's peer ID and add it to the bootstrap list"
echo "To find chandrayaan's peer ID:"
echo "  1. SSH into chandrayaan-mainnet.accumulate.defidevs.io"
echo "  2. Run: accumulated key export --key-type p2p"
echo "  3. Add: --peer /dns/chandrayaan-mainnet.accumulate.defidevs.io/tcp/16593/p2p/[PEER_ID]"
echo ""
echo "Checking container status..."
docker ps | grep bootstrap
echo ""
echo "Testing connectivity..."
sleep 5
docker logs --tail 10 accumulate-bootstrap 2>&1 | grep -E "(We are|peer)"
echo ""
echo "======================================"
echo "Bootstrap update complete"
echo "======================================"
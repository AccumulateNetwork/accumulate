# Accumulate Performance Test Guide

## Overview

This test suite measures Accumulate network throughput across different
topologies. It deploys validators and a bootstrap peer-discovery server
using Docker, then ramps transaction load from 1K to 25K TPS while
monitoring error rates, latency, and resource usage.

The suite is currently configured for single-host Docker deployment.
This document covers both that mode and how to adapt it for
multi-server deployment on Linux and Windows.

---

## Architecture

Every Accumulate validator runs in **dual mode**: it hosts both a
Directory Network Node (DNN) and a Block Validator Network Node (BVNN).
The test deploys these components:

```
                     +-------------------+
                     | Bootstrap Server  |  (peer discovery, libp2p DHT)
                     | port 16593        |
                     +--------+----------+
                              |
        +---------------------+----------------------+
        |                     |                      |
  +-----+------+       +-----+------+        +------+-----+
  |   BVN 1    |       |   BVN 2    |        |   BVN 3    |
  | val1  val2 |       | val1  val2 |        | val1  val2 |
  | val3  val4 |       | val3  val4 |        | val3  val4 |
  +------------+       +------------+        +------------+
        |                     |                      |
  +-----+---------------------+----------------------+
  |              Load Test Workers                     |
  |  (parallel-loadtest.go, distributed across nodes)  |
  +----------------------------------------------------+
```

### Topologies Tested

The suite tests 9 combinations of validators and BVNs:

| Topology | Validators | BVNs | Total Nodes |
|----------|-----------|------|-------------|
| 2v-1b    | 2         | 1    | 2           |
| 2v-2b    | 2         | 2    | 4           |
| 2v-3b    | 2         | 3    | 6           |
| 3v-1b    | 3         | 1    | 3           |
| 3v-2b    | 3         | 2    | 6           |
| 3v-3b    | 3         | 3    | 9           |
| 4v-1b    | 4         | 1    | 4           |
| 4v-2b    | 4         | 2    | 8           |
| 4v-3b    | 4         | 3    | 12          |

### Port Assignments

Each validator exposes its JSON-RPC API on a unique host port mapped
from container port 26660:

| Validator  | Host Port |
|------------|-----------|
| bvn1-val1  | 26660     |
| bvn1-val2  | 26661     |
| bvn1-val3  | 26662     |
| bvn1-val4  | 26663     |
| bvn2-val1  | 26664     |
| ...        | ...       |
| bvn3-val4  | 26671     |

The bootstrap server always uses port 16593.

---

## File Inventory

### Docker Compose Files

```
test/docker/
  docker-compose-{2,3,4}-val-{1,2,3}-bvn.yml   # 9 topology files
  docker-compose.yml                              # Default 12-node (4v x 3b)
  docker-compose.distributed.yml                  # Per-container isolation
```

### Network Configuration Files

```
test/docker/
  docker-network-{2,3,4}-val-{1,2,3}-bvn.yml    # 9 matching network configs
  docker-network.yml                               # Default 12-node config
```

Each network config defines:
- **Network ID** (e.g., `DAGBFTTest-2v-1b`)
- **Bootstrap** peer/advertise address and base port
- **Globals**: executor version, oracle price, major block schedule
- **BVN list**: each BVN lists its validator nodes with Docker hostnames

### Scripts

| File | Purpose |
|------|---------|
| `run-performance-suite.sh` | Runs all 9 topologies sequentially |
| `run-loadtest.sh`          | Runs a single topology test |
| `run-full-test.sh`         | Quick 10K TPS test (4 validators, 1 BVN) |
| `manage.sh`                | Start/stop/status/logs helper |

### Load Test and Monitoring

| File | Purpose |
|------|---------|
| `parallel-loadtest.go`  | Worker-based load generator |
| `dashboard-server.py`   | HTTP server for real-time dashboard |
| `dashboard.html`        | Browser-based metrics display |
| `monitoring.py`         | Per-container CPU/memory/disk collection |

### Binaries in Docker Image

The Dockerfile (repo root) produces:

| Binary | Purpose |
|--------|---------|
| `accumulated`           | Validator node (default entrypoint) |
| `accumulated-bootstrap` | Peer discovery server |
| `snapshot`              | Snapshot creation tool |
| `dbrepair`              | Database repair utility |
| `debug`                 | Debug utilities |
| `cometbft`              | Consensus engine |

---

## Running the Tests

### Prerequisites

- Docker with Compose v2
- Go 1.22+
- Python 3.8+ (for dashboard and monitoring)
- 16 GB RAM minimum (32 GB for 4v-3b topology)
- Ports 16593, 26660-26671 available

### Single Topology (Quick Test)

```bash
cd test/docker

# Default: 4 nodes, 32 workers, 120s, 5000 start TPS
./run-loadtest.sh

# Custom run
./run-loadtest.sh --duration 300s --workers 64 --start-tps 2000 --keep

# Teardown after --keep
./run-loadtest.sh --teardown
```

Dashboard at http://localhost:8888/ while running.

### Full 9-Topology Suite

```bash
cd test/docker

# Run all 9
./run-performance-suite.sh

# Run specific topologies (validator-count bvn-count pairs)
./run-performance-suite.sh 2 1 3 2 4 3
```

Results written to `test/docker/performance-results/`:
- `PERFORMANCE-RESULTS-<timestamp>.md` -- summary table
- `<topology>-output.txt` -- per-topology raw output
- `FAILED-<topology>-<timestamp>.txt` -- failure reports

### Load Test CLI Reference

`parallel-loadtest.go` accepts these flags:

| Flag | Default | Description |
|------|---------|-------------|
| `-nodes` | localhost:26660-26671 | Comma-separated API endpoints |
| `-workers` | 16 | Worker goroutines (min 16) |
| `-start-tps` | 1000 | Initial TPS target |
| `-max-tps` | 25000 | Maximum TPS target |
| `-ramp-step` | 2000 | TPS increase per ramp interval |
| `-ramp-interval` | 30s | Time between TPS increases |
| `-duration` | 5m | Total test duration |
| `-error-cutoff` | 5.0 | Stop ramping at this error % |
| `-accounts` | 1000 | Total sender accounts |
| `-fund-amount` | 100000 | ACME per funder account |
| `-oracle` | 1000 | ACME oracle price (USD) |
| `-faucet-seed` | FAUCET | Deterministic faucet key seed |
| `-label` | "" | Test label for dashboard |
| `-status-dir` | /tmp/loadtest-workspace | Directory for status.json |

### Dashboard

The dashboard shows real-time metrics:
- **Current TPS** (15-second sliding window)
- **Peak TPS** and average TPS
- **Total transactions** (submitted / succeeded / failed)
- **Error rate** with color coding (green < 1%, yellow < 5%, red > 5%)
- **Worker count** and node count
- **TPS history chart** (Chart.js, last 150 data points)
- **Test log** (last 200 JSONL entries)

Status is polled every 2 seconds from `/status` (JSON).
Logs are polled every 5 seconds from `/log` (JSON array).

### Monitoring

`monitoring.py` collects per-container metrics every 10 seconds:
- CPU percentage and memory usage (via `docker stats`)
- Database size on disk (via filesystem walk)
- Output: CSV files in `/tmp/loadtest-workspace/monitoring/`

Container discovery runs every iteration, so monitoring can start
before containers exist.

---

## Test Flow Detail

### What Happens During a Single Topology Test

1. **Cleanup**: Remove containers, volumes, networks from prior run
2. **Deploy**: `docker compose -f <topology>.yml up -d`
3. **Init**: The `init` container runs `accumulated init network <config>.yml --reset`,
   generating genesis state, validator keys, and configuration in the
   shared `network-config` volume
4. **Bootstrap**: `accumulated-bootstrap` starts independently, listening
   on port 16593 for peer discovery
5. **Validators**: Each validator starts after init completes and
   bootstrap is running. They load config from the shared volume and
   discover peers via bootstrap
6. **Health check**: Script polls the first validator's JSON-RPC API
   (`network-status` method) until it responds (up to 120s)
7. **Load test**: `parallel-loadtest.go` runs with the topology's
   API endpoints. Workers distribute evenly across nodes. TPS ramps
   from 1K upward, adding 2K every 30s, holding if error rate > 5%
8. **Results**: Final stats printed and parsed (peak TPS, avg TPS,
   submitted, failed)
9. **Teardown**: `docker compose down -v`, remove volumes

### How the Load Generator Works

Each worker:
1. Derives a deterministic funder account from the faucet seed
2. For each sender account slot:
   - First visit: fund from funder (SendTokens)
   - Second visit: buy credits (AddCredits)
   - Subsequent visits: send 1 token to a destination account
3. Destination walks use a coprime stride to avoid hot spots
4. Workers rate-limit to their share of the global TPS target
   (`target_tps / num_workers`)
5. The TPS target ramps up on a timer; ramp pauses if cumulative
   error rate exceeds the cutoff

---

## Adapting for Multi-Server Deployment

The single-host Docker setup simulates a distributed network. For
real multi-server testing, each server runs a subset of the
containers with network connectivity between them.

### Conceptual Architecture

```
  Server A (Linux)           Server B (Linux)         Server C (Windows)
  +-----------------+        +-----------------+      +-----------------+
  | Bootstrap       |        | BVN2 validators |      | BVN3 validators |
  | BVN1 validators |        |                 |      |                 |
  | Load workers    |        | Load workers    |      | Load workers    |
  | Dashboard       |        | Monitoring      |      | Monitoring      |
  +-----------------+        +-----------------+      +-----------------+
         |                          |                         |
         +------------- LAN / WAN ----------------------------+
```

### Step 1: Build and Distribute the Docker Image

Build once, push to a registry or transfer directly:

```bash
# On the build machine
cd /path/to/accumulate
docker build -t accumulated-test .

# Option A: Push to registry
docker tag accumulated-test registry.example.com/accumulated-test:latest
docker push registry.example.com/accumulated-test:latest

# Option B: Export and copy
docker save accumulated-test | gzip > accumulated-test.tar.gz
scp accumulated-test.tar.gz user@serverB:/tmp/
# On serverB:
docker load < /tmp/accumulated-test.tar.gz
```

On Windows servers, use Docker Desktop with WSL2 backend or Docker
Engine on Windows Server 2019+. The same Linux container image works
under WSL2.

### Step 2: Create Multi-Server Network Configuration

Create a new `docker-network-multi.yml` replacing Docker hostnames
with actual server IPs:

```yaml
id: "PerfTest-Multi"

bootstrap:
  peerAddress: "192.168.1.10"      # Server A IP
  advertizeAddress: "192.168.1.10"
  listenAddress: "0.0.0.0"
  basePort: 16593

globals:
  executorVersion: "v2"
  oracle:
    price: 10000000
  globals:
    majorBlockSchedule: "0 */12 * * *"

bvns:
  - id: "BVN1"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "192.168.1.10"       # Server A
        advertizeAddress: "192.168.1.10"
        basePort: 26656
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "0.0.0.0"
        peerAddress: "192.168.1.10"       # Server A (second validator)
        advertizeAddress: "192.168.1.10"
        basePort: 26756                    # Different base port on same host
        dnnType: "validator"
        bvnnType: "validator"

  - id: "BVN2"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "192.168.1.11"       # Server B
        advertizeAddress: "192.168.1.11"
        basePort: 26656
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "0.0.0.0"
        peerAddress: "192.168.1.11"
        advertizeAddress: "192.168.1.11"
        basePort: 26756
        dnnType: "validator"
        bvnnType: "validator"

  - id: "BVN3"
    nodes:
      - listenAddress: "0.0.0.0"
        peerAddress: "192.168.1.12"       # Server C (Windows)
        advertizeAddress: "192.168.1.12"
        basePort: 26656
        dnnType: "validator"
        bvnnType: "validator"
      - listenAddress: "0.0.0.0"
        peerAddress: "192.168.1.12"
        advertizeAddress: "192.168.1.12"
        basePort: 26756
        dnnType: "validator"
        bvnnType: "validator"
```

Key differences from single-host:
- `peerAddress` and `advertizeAddress` are real IPs, not Docker hostnames
- Multiple validators on the same host use different `basePort` values
- Ports must be open in firewalls between servers

### Step 3: Create Per-Server Compose Files

Each server gets its own compose file running only its validators.
Use `network_mode: host` so containers bind directly to the host
network (no Docker NAT).

**Server A** (`docker-compose-server-a.yml`):

```yaml
services:
  bootstrap:
    image: accumulated-test
    container_name: acc-bootstrap
    network_mode: host
    entrypoint: ["/bin/accumulated-bootstrap"]
    command:
      - --listen
      - /ip4/0.0.0.0/tcp/16593
      - --key
      - "626f6f7473747261702d6e6f64652d6b65792d736565642d7631000000000000"
    restart: unless-stopped

  init:
    image: accumulated-test
    container_name: acc-init
    volumes:
      - network-config:/root/.accumulate
      - /path/to/accumulate:/workspace
    working_dir: /workspace
    command:
      - init
      - network
      - docker-network-multi.yml
      - --reset
      - --faucet-seed=FAUCET

  bvn1-val1:
    image: accumulated-test
    container_name: acc-bvn1-val1
    network_mode: host
    volumes:
      - network-config:/root/.accumulate
    command:
      - -w=/root/.accumulate/bvn1-1
      - /root/.accumulate/bvn1-1/accumulate.toml
    depends_on:
      init:
        condition: service_completed_successfully

  bvn1-val2:
    image: accumulated-test
    container_name: acc-bvn1-val2
    network_mode: host
    volumes:
      - network-config:/root/.accumulate
    command:
      - -w=/root/.accumulate/bvn1-2
      - /root/.accumulate/bvn1-2/accumulate.toml
    depends_on:
      init:
        condition: service_completed_successfully

volumes:
  network-config:
```

**Server B and C**: Same pattern but with their BVN's validators.
The init container only needs to run on ONE server. Copy the
generated config volume to other servers:

```bash
# On Server A (after init completes)
docker run --rm -v network-config:/data -v /tmp:/backup \
  alpine tar czf /backup/network-config.tar.gz -C /data .

scp /tmp/network-config.tar.gz user@serverB:/tmp/

# On Server B
docker volume create network-config
docker run --rm -v network-config:/data -v /tmp:/backup \
  alpine tar xzf /backup/network-config.tar.gz -C /data
```

### Step 4: Open Firewall Ports

Each server needs these ports accessible from all other servers:

| Port Range | Protocol | Purpose |
|------------|----------|---------|
| 16593      | TCP      | Bootstrap peer discovery |
| 26656-26659| TCP      | Tendermint P2P consensus |
| 26660-26663| TCP      | JSON-RPC API |
| 26756-26759| TCP      | Second validator P2P (if 2+ per host) |
| 26760-26763| TCP      | Second validator API |

On Windows, use PowerShell:
```powershell
New-NetFirewallRule -DisplayName "Accumulate P2P" `
  -Direction Inbound -Protocol TCP `
  -LocalPort 16593,26656-26663,26756-26763 -Action Allow
```

On Linux:
```bash
for port in 16593 26656:26663 26756:26763; do
  sudo ufw allow $port/tcp
done
```

### Step 5: Start the Network

```bash
# Server A (has init + bootstrap + BVN1)
docker compose -f docker-compose-server-a.yml up -d

# Wait for init to complete and bootstrap to start
# Then copy network-config to other servers (Step 3)

# Server B (BVN2 only, no init needed)
docker compose -f docker-compose-server-b.yml up -d

# Server C - Windows (BVN3 only)
docker compose -f docker-compose-server-c.yml up -d
```

### Step 6: Run the Load Test

Run the load generator from any server that can reach all API endpoints:

```bash
go run test/docker/parallel-loadtest.go \
  -nodes "http://192.168.1.10:26660/v3,http://192.168.1.10:26661/v3,http://192.168.1.11:26660/v3,http://192.168.1.11:26661/v3,http://192.168.1.12:26660/v3,http://192.168.1.12:26661/v3" \
  -workers 48 \
  -start-tps 2000 \
  -max-tps 30000 \
  -duration 10m \
  -label "Multi-server 3x2"
```

Start the dashboard on the same machine:

```bash
python3 test/docker/dashboard-server.py 8888
```

### Step 7: Distributed Monitoring

Run `monitoring.py` on each server to collect local container metrics.
Aggregate the CSV files after the test:

```bash
# On each server
python3 test/docker/monitoring.py &

# After test, collect from all servers
scp serverB:/tmp/loadtest-workspace/monitoring/*.csv ./results/serverB/
scp serverC:/tmp/loadtest-workspace/monitoring/*.csv ./results/serverC/
```

---

## Windows-Specific Notes

### Docker Setup

Windows Server 2019+ supports Linux containers via:
1. **Docker Desktop with WSL2** (recommended for dev/test)
2. **Docker Engine** on Windows Server with Hyper-V isolation

The Accumulate Docker image is Linux-based (Alpine). It runs
natively under WSL2 or Hyper-V Linux container mode.

```powershell
# Verify Linux containers are enabled
docker info | Select-String "OSType"
# Should show: OSType: linux
```

### Network Mode

`network_mode: host` does not work on Docker Desktop for Windows.
Instead, use explicit port mappings and ensure the Windows firewall
allows the mapped ports:

```yaml
  bvn3-val1:
    image: accumulated-test
    ports:
      - "26660:26660"
      - "26656:26656"
    # ... rest of config
```

The `advertizeAddress` in the network config must be the Windows
host's LAN IP, not `localhost` or `127.0.0.1`.

### Path Volumes

Windows paths in Docker Compose use forward slashes or escaped
backslashes:

```yaml
volumes:
  - C:/accumulate/network-config:/root/.accumulate
  # or
  - //c/accumulate/network-config:/root/.accumulate
```

---

## Tuning for Multi-Server Tests

### Network Latency

Real servers introduce network latency that single-host Docker
does not. Expected effects:
- Consensus rounds take longer (proportional to round-trip time)
- TPS ceiling drops with higher latency between validators
- Cross-BVN synthetic transactions have additional hop delay

To simulate latency on single-host Docker before deploying:
```bash
# Add 10ms latency to a container's network
docker exec acc-bvn1-val1 tc qdisc add dev eth0 root netem delay 10ms
```

### Resource Limits

Adjust per-server based on available resources:

| Topology | RAM per Server | CPU Cores |
|----------|---------------|-----------|
| 2 validators/server | 8 GB | 4 |
| 4 validators/server | 16 GB | 8 |
| 6 validators/server | 24 GB | 12 |

Each validator uses approximately 1-2 GB RAM under load.

### Load Generator Placement

For accurate throughput measurement, run load generators on
separate machines from validators, or at minimum distribute workers
evenly across servers:

```bash
# Machine D (dedicated load generator)
go run parallel-loadtest.go \
  -nodes "http://serverA:26660/v3,...,http://serverC:26661/v3" \
  -workers 64 \
  -start-tps 5000 \
  -max-tps 50000 \
  -duration 15m
```

If running load generators on validator servers, reduce worker count
to avoid starving validators of CPU.

---

## Troubleshooting

### Validators Stuck in "Created" State

The bootstrap server must be running before validators start.
Check:
```bash
docker logs acc-bootstrap
# Should show: "Listening on /ip4/0.0.0.0/tcp/16593"
```

If bootstrap fails, validators will never start because they
depend on it.

### Network Not Reaching Healthy

Validators need to discover each other via bootstrap. Verify:
```bash
# Check if API responds
curl -s -X POST http://localhost:26660/v3 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{}}'
```

Common causes:
- Firewall blocking P2P ports (26656+)
- Wrong `peerAddress`/`advertizeAddress` in network config
- Init did not complete (check `docker logs acc-init`)
- Volume not shared correctly between init and validators

### High Error Rate During Load Test

Error rate > 5% causes TPS ramp to pause. Common causes:
- Validators overloaded (check CPU via `docker stats`)
- Network congestion between servers
- Insufficient credits on sender accounts (increase `-fund-amount`)

### Dashboard Shows "Disconnected"

The dashboard reads `/tmp/loadtest-workspace/status.json`. If the
load test is not running or crashed, the file goes stale (> 5s old)
and the dashboard shows "Stale". Check:
```bash
cat /tmp/loadtest-workspace/status.json
ls -la /tmp/loadtest-workspace/status.json
```

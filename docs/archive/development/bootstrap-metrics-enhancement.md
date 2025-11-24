# Bootstrap Server Metrics Enhancement

## Current State

Port 8081 IS configured but only provides basic Go runtime metrics:
- `go_goroutines` - Number of goroutines
- `go_memstats_*` - Memory statistics
- `go_gc_*` - Garbage collection stats

## Missing Information

No bootstrap-specific metrics:
- ❌ DHT routing table size
- ❌ Number of connected peers
- ❌ Bootstrap requests served
- ❌ Peer discovery statistics
- ❌ Connection health
- ❌ Network reachability

## Enhancement Plan

### 1. Enable libp2p Built-in Metrics

libp2p provides Prometheus metrics out of the box. We just need to enable them.

**Code change in `cmd/accumulated-bootstrap/main.go`:**

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    libp2p "github.com/libp2p/go-libp2p"
)

func run(*cobra.Command, []string) {
    // Create Prometheus registry for libp2p
    reg := prometheus.NewRegistry()

    cfg := &Config{
        Instrumentation: &Instrumentation{
            HttpListener: HttpListener{
                Listen: flag.PromListen,
            },
        },
        P2P: &P2P{
            Key:            flag.Key.Value,
            Listen:         flag.Listen,
            BootstrapPeers: flag.Peers,
            DiscoveryMode:  Ptr(DhtMode(dht.ModeAutoServer)),
            External:       flag.External,

            // Enable libp2p metrics
            Options: []libp2p.Option{
                libp2p.PrometheusRegisterer(reg),
            },
        },
    }

    ctx := ContextForMainProcess(context.Background())
    inst, err := Start(ctx, cfg)
    Check(err)
    <-inst.Done()
    inst.Stop()
}
```

**Metrics this would expose:**
- `libp2p_peers` - Number of connected peers
- `libp2p_network_bytes_sent_total` - Total bytes sent
- `libp2p_network_bytes_received_total` - Total bytes received
- `libp2p_tcp_connections` - TCP connections
- `libp2p_dht_*` - DHT metrics if available

### 2. Add Custom Bootstrap Info Endpoint

Add a `/info` endpoint that returns JSON with bootstrap server status.

**Code addition to `cmd/accumulated-bootstrap/main.go`:**

```go
import (
    "encoding/json"
    "net/http"
)

type BootstrapInfo struct {
    PeerID            string   `json:"peer_id"`
    ListenAddresses   []string `json:"listen_addresses"`
    ExternalAddresses []string `json:"external_addresses"`
    DHT               DHTInfo  `json:"dht"`
    Connections       ConnInfo `json:"connections"`
    UptimeSeconds     int64    `json:"uptime_seconds"`
}

type DHTInfo struct {
    Mode              string `json:"mode"`
    RoutingTableSize  int    `json:"routing_table_size"`
}

type ConnInfo struct {
    Total    int `json:"total"`
    Inbound  int `json:"inbound"`
    Outbound int `json:"outbound"`
}

func infoHandler(inst *Instance) http.HandlerFunc {
    startTime := time.Now()

    return func(w http.ResponseWriter, r *http.Request) {
        host := inst.p2p.(*p2p.Node).Host()

        info := BootstrapInfo{
            PeerID:            host.ID().String(),
            ListenAddresses:   addrsToStrings(host.Addrs()),
            ExternalAddresses: addrsToStrings(host.Network().Peers()),
            DHT: DHTInfo{
                Mode:             "server",
                RoutingTableSize: len(host.Network().Peers()),
            },
            Connections: ConnInfo{
                Total:    len(host.Network().Conns()),
                Inbound:  countInbound(host.Network().Conns()),
                Outbound: countOutbound(host.Network().Conns()),
            },
            UptimeSeconds: int64(time.Since(startTime).Seconds()),
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(info)
    }
}

// Register the handler when starting HTTP server
// Add to instrumentation.go:listen() function after line 55
mux := http.NewServeMux()
mux.Handle("/metrics", promhttp.InstrumentMetricHandler(...))
mux.HandleFunc("/info", infoHandler(inst))
mux.HandleFunc("/health", healthHandler(inst))
```

### 3. Add Health Check Endpoint

Simple endpoint for monitoring systems.

```go
func healthHandler(inst *Instance) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        host := inst.p2p.(*p2p.Node).Host()

        // Check if we have any peers
        if len(host.Network().Peers()) == 0 {
            w.WriteHeader(http.StatusServiceUnavailable)
            json.NewEncoder(w).Encode(map[string]string{
                "status": "unhealthy",
                "reason": "no peers in DHT routing table",
            })
            return
        }

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "healthy",
        })
    }
}
```

## Expected Endpoints After Enhancement

```yaml
endpoints:
  metrics:
    path: "/metrics"
    format: "prometheus"
    content:
      - "Go runtime metrics (existing)"
      - "libp2p_peers - Connected peers"
      - "libp2p_network_bytes_* - Network traffic"
      - "libp2p_tcp_connections - TCP connections"
      - "libp2p_dht_* - DHT metrics"

  info:
    path: "/info"
    format: "json"
    content:
      - "peer_id - Bootstrap server peer ID"
      - "listen_addresses - Internal listen addresses"
      - "external_addresses - Advertised addresses"
      - "dht - DHT routing table stats"
      - "connections - Connection statistics"
      - "uptime_seconds - Server uptime"

  health:
    path: "/health"
    format: "json"
    content:
      - "status - healthy|unhealthy"
      - "reason - Details if unhealthy"
```

## Example Usage

### Query Prometheus Metrics
```bash
curl http://bootstrap.accumulate.defidevs.io:8081/metrics

# Output:
# go_goroutines 42
# libp2p_peers 127
# libp2p_network_bytes_sent_total 1234567
# libp2p_tcp_connections{direction="inbound"} 45
```

### Query Bootstrap Info
```bash
curl http://bootstrap.accumulate.defidevs.io:8081/info | jq

# Output:
{
  "peer_id": "12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx",
  "listen_addresses": [
    "/ip4/0.0.0.0/tcp/16593",
    "/ip4/0.0.0.0/tcp/16693"
  ],
  "external_addresses": [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3Koo...",
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16693/p2p/12D3Koo..."
  ],
  "dht": {
    "mode": "server",
    "routing_table_size": 127
  },
  "connections": {
    "total": 89,
    "inbound": 45,
    "outbound": 44
  },
  "uptime_seconds": 86400
}
```

### Health Check
```bash
curl http://bootstrap.accumulate.defidevs.io:8081/health

# Output:
{
  "status": "healthy"
}
```

## Implementation Effort

```yaml
effort:
  libp2p_metrics:
    difficulty: "low"
    changes_required:
      - "Add libp2p.PrometheusRegisterer option to P2P config"
      - "Pass registry to instrumentation handler"
    lines_of_code: "~20"

  info_endpoint:
    difficulty: "medium"
    changes_required:
      - "Create info handler function"
      - "Query libp2p host for stats"
      - "Register handler with HTTP mux"
    lines_of_code: "~100"

  health_endpoint:
    difficulty: "low"
    changes_required:
      - "Create simple health check"
      - "Register handler with HTTP mux"
    lines_of_code: "~30"

  total:
    estimated_lines: "~150"
    files_modified: 2-3
    deployment_scope: "ONE bootstrap server"
    requires_network_update: false
```

## Deployment After Changes

After implementing these enhancements:

1. **Build new binary:**
   ```bash
   cd cmd/accumulated-bootstrap
   go build -o accumulated-bootstrap
   ```

2. **Deploy to bootstrap server:**
   ```bash
   # SSH to bootstrap.accumulate.defidevs.io
   # Stop current server
   docker stop accumulate-bootstrap

   # Run with new binary
   docker run -d \
     --name accumulate-bootstrap \
     --restart unless-stopped \
     -p 16593:16593 \
     -p 16693:16693 \
     -p 8081:8081 \
     -v /opt/accumulate/bootstrap-key.txt:/key.txt:ro \
     registry.gitlab.com/accumulatenetwork/accumulate:latest \
     accumulated-bootstrap \
     --key /key.txt \
     --listen /ip4/0.0.0.0/tcp/16593 \
     --listen /ip4/0.0.0.0/tcp/16693 \
     --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
     --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693 \
     --prom-listen /ip4/0.0.0.0/tcp/8081/http
   ```

3. **Verify metrics:**
   ```bash
   # Check Prometheus metrics
   curl http://bootstrap.accumulate.defidevs.io:8081/metrics | grep libp2p

   # Check info endpoint
   curl http://bootstrap.accumulate.defidevs.io:8081/info | jq

   # Check health
   curl http://bootstrap.accumulate.defidevs.io:8081/health
   ```

## Benefits

```yaml
benefits:
  monitoring:
    - "Track bootstrap server health"
    - "Alert on connection issues"
    - "Monitor DHT routing table size"

  debugging:
    - "Verify bootstrap server is reachable"
    - "Check peer discovery is working"
    - "Diagnose connection problems"

  visibility:
    - "See how many nodes are connecting"
    - "Track network growth"
    - "Measure bootstrap effectiveness"

  integration:
    - "MCP server can query live stats"
    - "Automated health checks"
    - "Grafana dashboards for visualization"
```

## MCP Server Integration

After adding these endpoints, the MCP server could provide:

```yaml
new_mcp_tool:
  name: "accumulate_query_bootstrap_server"
  purpose: "Get live bootstrap server statistics"
  inputs:
    server: "bootstrap.accumulate.defidevs.io"
  outputs:
    peer_id: "Bootstrap peer ID"
    listen_addresses: "Listening addresses"
    dht_size: "DHT routing table size"
    connections: "Active connections"
    health: "Server health status"
```

## Summary

Port 8081 is configured but underutilized. With ~150 lines of code:
1. Enable libp2p Prometheus metrics (low effort)
2. Add `/info` endpoint for bootstrap stats (medium effort)
3. Add `/health` endpoint for monitoring (low effort)

This would provide valuable visibility into bootstrap server operation without requiring network-wide deployment.

# Bootstrap Server Info Server Implementation

## Summary

Added HTTP info server on port 8080 with `/info` and `/health` endpoints for monitoring and debugging the bootstrap server.

## Changes Made

### New Files

**`cmd/accumulated-bootstrap/info_server.go`** (230 lines)
- `InfoServer` struct for serving bootstrap information
- `/info` endpoint returning detailed server stats
- `/health` endpoint for health monitoring
- JSON response formatting

### Modified Files

**`cmd/accumulated-bootstrap/main.go`**
- Added `--info-listen` flag (default: `/ip4/0.0.0.0/tcp/8080/http`)
- Integrated InfoServer startup in run() function
- Graceful shutdown handling

**`pkg/api/v3/p2p/p2p.go`**
- Added `Host() host.Host` method to expose libp2p host

**`cmd/accumulated/run/instance.go`**
- Added `P2P() *p2p.Node` accessor method

## Endpoints

### GET /info

Returns detailed bootstrap server information in JSON format.

**Response:**
```json
{
  "peer_id": "12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx",
  "listen_addresses": [
    "/ip4/0.0.0.0/tcp/16593",
    "/ip4/0.0.0.0/tcp/16693"
  ],
  "external_addresses": [
    "/ip4/3.138.61.111/tcp/16593/p2p/12D3KooWDgqY...",
    "/ip4/3.138.61.111/tcp/16693/p2p/12D3KooWDgqY..."
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

**Fields:**
- `peer_id`: libp2p peer ID of the bootstrap server
- `listen_addresses`: Internal listen addresses (without peer ID)
- `external_addresses`: Full multiaddr with peer ID for connecting
- `dht.mode`: DHT mode (always "server")
- `dht.routing_table_size`: Number of peers in DHT routing table
- `connections.total`: Total active connections
- `connections.inbound`: Inbound connections
- `connections.outbound`: Outbound connections
- `uptime_seconds`: Server uptime in seconds

### GET /health

Returns health status for monitoring systems.

**Response (Healthy):**
```json
{
  "status": "healthy",
  "peer_count": 127,
  "conn_count": 89,
  "uptime_hours": 24
}
```

**Response (Unhealthy):**
```json
{
  "status": "unhealthy",
  "reason": "no peers in DHT routing table after 5 minutes",
  "peer_count": 0,
  "conn_count": 0,
  "uptime_hours": 1
}
```

**HTTP Status Codes:**
- `200 OK`: Server is healthy
- `503 Service Unavailable`: Server is unhealthy

**Health Criteria:**
- Considered unhealthy if no peers after 5 minutes of uptime
- Grace period allows for initial bootstrap

## Usage

### Command Line

```bash
# Default (port 8080)
accumulated-bootstrap \
  --key /path/to/key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693

# Custom info port
accumulated-bootstrap \
  --key /path/to/key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --info-listen /ip4/0.0.0.0/tcp/9000/http
```

### Docker

```bash
docker run -d \
  --name accumulate-bootstrap \
  --restart unless-stopped \
  -p 16593:16593 \
  -p 16693:16693 \
  -p 8080:8080 \
  -v /opt/accumulate/bootstrap-key.txt:/key.txt:ro \
  registry.gitlab.com/accumulatenetwork/accumulate:latest \
  accumulated-bootstrap \
  --key /key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693
```

Note: Port 8080 is exposed by default. No additional flags needed.

### Querying Endpoints

```bash
# Get server info
curl http://bootstrap.accumulate.defidevs.io:8080/info | jq

# Check health
curl http://bootstrap.accumulate.defidevs.io:8080/health

# Monitor health (exit on failure)
curl -f http://bootstrap.accumulate.defidevs.io:8080/health || echo "Server unhealthy!"
```

## Testing

### Test Results

```bash
$ timeout 10s ./accumulated-bootstrap \
  --listen /ip4/127.0.0.1/tcp/26593 \
  --listen /ip4/127.0.0.1/tcp/26693 \
  --info-listen /ip4/127.0.0.1/tcp/28080/http \
  2>&1 &

$ curl -s http://127.0.0.1:28080/info
{
  "peer_id": "12D3KooWJiS4qWQuMC1ikaEHtX1NbSupP3cmQm4iu4PAouedhgv2",
  "listen_addresses": [
    "/ip4/127.0.0.1/tcp/26593",
    "/ip4/127.0.0.1/tcp/26693"
  ],
  "external_addresses": [
    "/ip4/127.0.0.1/tcp/26593/p2p/12D3KooWJiS...",
    "/ip4/127.0.0.1/tcp/26693/p2p/12D3KooWJiS..."
  ],
  "dht": {
    "mode": "server",
    "routing_table_size": 0
  },
  "connections": {
    "total": 0,
    "inbound": 0,
    "outbound": 0
  },
  "uptime_seconds": 2
}

$ curl -s http://127.0.0.1:28080/health
{
  "conn_count": 0,
  "peer_count": 0,
  "status": "healthy",
  "uptime_hours": 0
}
```

## Security Considerations

### Port 8080 (Info Server)
- **Public-facing**: Safe to expose publicly
- **Read-only**: No write operations
- **Non-sensitive data**: Only exposes public network information
- **Rate limiting**: Uses standard HTTP timeouts (10s header, 30s read/write)
- **Idle timeout**: 120 seconds

### Port 8081 (Prometheus)
- **Internal use**: Should NOT be exposed publicly without authentication
- **Disabled by default in production**: Requires explicit `--prom-listen` flag
- **DoS risk**: Metrics generation can be CPU-intensive
- **Recommendation**: Keep on internal network or add auth proxy

## Monitoring Integration

### Prometheus/Alertmanager

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'accumulate-bootstrap'
    metrics_path: '/health'
    static_configs:
      - targets: ['bootstrap.accumulate.defidevs.io:8080']
```

### Nagios/Icinga

```bash
#!/bin/bash
# check_accumulate_bootstrap.sh

STATUS=$(curl -sf http://bootstrap.accumulate.defidevs.io:8080/health | jq -r '.status')

if [ "$STATUS" = "healthy" ]; then
  echo "OK - Bootstrap server healthy"
  exit 0
else
  REASON=$(curl -sf http://bootstrap.accumulate.defidevs.io:8080/health | jq -r '.reason')
  echo "CRITICAL - Bootstrap server unhealthy: $REASON"
  exit 2
fi
```

### Kubernetes Liveness/Readiness

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: accumulate-bootstrap
spec:
  containers:
  - name: bootstrap
    image: registry.gitlab.com/accumulatenetwork/accumulate:latest
    livenessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 30
      periodSeconds: 10
    readinessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 5
```

## MCP Server Integration

The MCP server can now query live bootstrap server stats:

```go
// New MCP tool: accumulate_query_bootstrap_info
func (s *Server) queryBootstrapInfo(args map[string]interface{}) (map[string]interface{}, error) {
    server, _ := args["server"].(string)
    if server == "" {
        server = "bootstrap.accumulate.defidevs.io"
    }

    resp, err := http.Get(fmt.Sprintf("http://%s:8080/info", server))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var info map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
        return nil, err
    }

    return info, nil
}
```

## Comparison with Previous State

### Before
- ❌ No way to query bootstrap server status
- ❌ No health check endpoint
- ❌ Prometheus port 8081 exposed (DoS risk)
- ❌ No connection statistics
- ❌ No uptime information

### After
- ✅ `/info` endpoint with comprehensive stats
- ✅ `/health` endpoint for monitoring
- ✅ Port 8080 (info) separate from 8081 (prometheus)
- ✅ Real-time connection counts
- ✅ DHT routing table size
- ✅ Uptime tracking
- ✅ Graceful shutdown handling
- ✅ Safe for public exposure

## Deployment Checklist

For deploying to production bootstrap server:

- [ ] Build new binary: `go build cmd/accumulated-bootstrap`
- [ ] Test locally with both ports 16593 and 16693
- [ ] Test info endpoint: `curl http://localhost:8080/info`
- [ ] Test health endpoint: `curl http://localhost:8080/health`
- [ ] Deploy to bootstrap.accumulate.defidevs.io
- [ ] Open firewall port 8080 (if needed)
- [ ] Verify endpoints accessible externally
- [ ] Add to monitoring system
- [ ] Update documentation

## Future Enhancements

Potential improvements for future versions:

1. **Metrics endpoint**: Add `/metrics` on port 8080 with basic Prometheus metrics
2. **Peer list**: Add `/peers` endpoint to list connected peers
3. **Query stats**: Track and expose DHT query statistics
4. **Historical data**: Add simple time-series data (last hour/day)
5. **WebSocket**: Real-time updates for monitoring dashboards
6. **Authentication**: Optional API key for sensitive operations

## References

- Source: `cmd/accumulated-bootstrap/info_server.go`
- Deployment: `cmd/accumulated-bootstrap/DEPLOYMENT.md`
- Quick fix: `cmd/accumulated-bootstrap/QUICK_FIX.md`
- Bootstrap architecture: `mcp/BOOTSTRAP_ARCHITECTURE.md`

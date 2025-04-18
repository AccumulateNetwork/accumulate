# Standalone Peer Discovery Package

## Overview

The `peerdiscovery` package provides a self-contained implementation of peer discovery logic for the Accumulate Network. It extracts host information from various address formats and constructs RPC endpoints for network communication.

## Installation

```bash
go get gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/new_heal/peerdiscovery
```

## Usage

```go
import (
    "log"
    "os"
    
    "gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/new_heal/peerdiscovery"
)

func main() {
    // Create a logger
    logger := log.New(os.Stdout, "[PeerDiscovery] ", log.LstdFlags)
    
    // Create a new peer discovery instance
    discovery := peerdiscovery.New(logger)
    
    // Extract a host from a multiaddr
    addr := "/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY"
    host, method := discovery.ExtractHost(addr)
    
    // Construct an RPC endpoint
    if host != "" {
        endpoint := discovery.GetPeerRPCEndpoint(host)
        logger.Printf("Extracted host %s using method %s", host, method)
        logger.Printf("Constructed endpoint: %s", endpoint)
    }
}
```

## API Reference

### PeerDiscovery

```go
type PeerDiscovery struct {
    logger *log.Logger
}
```

The main struct that provides peer discovery functionality.

### New

```go
func New(logger *log.Logger) *PeerDiscovery
```

Creates a new PeerDiscovery instance with the specified logger.

### ExtractHostFromMultiaddr

```go
func (p *PeerDiscovery) ExtractHostFromMultiaddr(addrStr string) (string, error)
```

Extracts a host from a multiaddr string. Returns the host and an error if extraction fails.

### ExtractHostFromURL

```go
func (p *PeerDiscovery) ExtractHostFromURL(urlStr string) (string, error)
```

Extracts a host from a URL string. Returns the host and an error if extraction fails.

### LookupValidatorHost

```go
func (p *PeerDiscovery) LookupValidatorHost(validatorID string) (string, bool)
```

Returns a known host for a validator ID. Returns the host and a boolean indicating if the validator was found.

### GetPeerRPCEndpoint

```go
func (p *PeerDiscovery) GetPeerRPCEndpoint(host string) string
```

Constructs an RPC endpoint from a host.

### ExtractHost

```go
func (p *PeerDiscovery) ExtractHost(addr string) (string, string)
```

Attempts to extract a host using all available methods. Returns the host and a string indicating which method was successful.

## Address Format Support

The package supports extracting hosts from the following address formats:

1. **Multiaddr Formats**:
   - IPv4: `/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY`
   - IPv6: `/ip6/2a01:4f9:3a:2c26::2/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp`
   - DNS: `/dns4/validator.lunanova.acme/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE`
   - UDP/QUIC: `/ip4/65.108.201.154/udp/16593/quic/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp`

2. **URL Formats**:
   - Standard URLs: `http://65.108.73.121:16592`
   - Host:Port: `validator.lunanova.acme:16592`
   - Plain IP: `65.108.4.175`

3. **Validator IDs**:
   - Validator names: `defidevs.acme`, `lunanova.acme`, etc.

## Edge Cases and Handling

1. **P2P-only Addresses**:
   - Format: `/p2p/defidevs.acme`
   - Handling: Falls back to validator ID mapping

2. **Invalid Multiaddrs**:
   - If multiaddr parsing fails, falls back to URL parsing
   - If URL parsing fails, falls back to validator ID mapping

3. **Missing Scheme in URLs**:
   - Automatically adds `http://` prefix if no scheme is present

## Performance Considerations

1. **Caching**:
   - Consider caching extraction results to avoid redundant processing
   - Use a map with the address as the key and the extracted host as the value

2. **Prioritization**:
   - The extraction methods are tried in order of reliability:
     1. Multiaddr parsing (fastest and most reliable for valid addresses)
     2. URL parsing (good fallback for non-multiaddr formats)
     3. Validator ID mapping (last resort for special cases)

3. **Concurrency**:
   - The package is thread-safe when used with different PeerDiscovery instances
   - Consider using goroutines for processing multiple addresses in parallel

## Integration with AddressDir

See the main peer discovery documentation for details on integrating this package with the AddressDir implementation.

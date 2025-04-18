# PeerState Implementation Status

## Overview

This document describes the current state of the PeerState implementation, which is designed to track and monitor the state of network peers in the Accumulate network. It works alongside the existing AddressDir struct to provide comprehensive network monitoring capabilities.

## Current Implementation Status

### What's Working

1. **Basic Structure**: The core PeerState struct and related types have been implemented according to the design documents.
2. **Network Peer Discovery**: The AddressDir struct can successfully discover network peers using the NetworkStatus API.
3. **Logging**: We've added detailed logging to track how peer addresses are processed and how RPC endpoints are constructed.
4. **Known Address Mapping**: We've implemented a hardcoded mapping for validator addresses as a fallback when direct host extraction fails.
5. **Command-Line Tool**: We've created a peer_state_cmd.go tool to test the PeerState functionality.

### Current Issues

1. **Host Extraction Challenges**: 
   - The implementation is struggling to extract valid host addresses from peer information.
   - Many peers have addresses that don't contain valid IP information, resulting in "Could not extract host from any address for peer" messages.
   - The multiaddress parsing logic needs improvement to handle more formats.

2. **Connection Failures**:
   - For peers where we do extract an address, we're seeing connection failures like "connection refused" when trying to connect to the RPC endpoints.
   - This suggests that either the extracted addresses are incorrect or the nodes are not accepting connections on the expected ports.

3. **Multiaddr Parsing Errors**:
   - We're encountering errors parsing multiaddresses that contain domain names instead of just peer IDs.
   - Example error: `invalid value "defidevs.acme" for protocol p2p: failed to parse p2p addr: defidevs.acme invalid cid: selected encoding not supported`

4. **Height Collection**:
   - Due to the connection issues, we're not successfully collecting height information from nodes.
   - This prevents us from identifying lagging nodes and zombie nodes as intended.

## Implementation Details

### Host Extraction Process

The current host extraction process follows these steps:

1. **Known Address Lookup**: First, try to get a known address for validators using the hardcoded mapping in `getKnownAddressesForValidator`.
2. **Direct IP Check**: Check if any of the peer's addresses are simple IP addresses (not multiaddresses).
3. **Multiaddr Parsing**: Try to parse each address as a multiaddress and extract the IP component.
4. **Fallback**: If no valid host can be extracted, log a message and return an empty string.

### RPC Endpoint Construction

Once a host is extracted, we construct an RPC endpoint using the following format:
```
http://{host}:16592
```

We try both port 16592 (primary) and 16692 (secondary) when querying node status.

### Height Collection Process

The height collection process is designed to:

1. Connect to each peer's RPC endpoint.
2. Query the node's status to get the latest block height.
3. Determine which partition the height belongs to (DN or BVN).
4. Update the appropriate height field in the peer's state information.
5. Check if the node is a zombie by examining its consensus participation.

However, due to the connection issues, this process is not currently working as expected.

## Next Steps

1. **Improve Host Extraction**:
   - Enhance the multiaddress parsing logic to handle more formats.
   - Add support for domain name resolution when IP addresses are not directly available.
   - Implement more robust fallback mechanisms when direct extraction fails.

2. **Test with Known Working Endpoints**:
   - Identify a set of known working endpoints to test the core functionality.
   - Verify that the height collection process works correctly with these endpoints.

3. **Add More Robust Error Handling**:
   - Implement better error handling and retry logic for connection failures.
   - Add timeouts to prevent hanging when connections are slow or unresponsive.

4. **Update Design Documents**:
   - Update the design documents to reflect the challenges encountered and solutions implemented.
   - Document the known address mapping approach as a key component of the implementation.

5. **Investigate Alternative Approaches**:
   - Look at how the original network status command successfully extracts and uses multiaddresses.
   - Consider implementing a similar approach in the PeerState implementation.

## Conclusion

The PeerState implementation has made significant progress but still faces challenges with host extraction and connection establishment. By addressing these issues, we can create a robust system for monitoring network peers and identifying lagging or zombie nodes.

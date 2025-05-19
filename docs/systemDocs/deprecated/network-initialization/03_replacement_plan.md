# Network Initiation Replacement

```yaml
# AI-METADATA
document_type: implementation
project: accumulate_network
component: debug_tools
topic: network_initiation_replacement
version: 1.0
date: 2025-05-02
```

## Divine Mantra

> "We do not modify how the network initialization is done. No improvements, no performance enhancements, no simplifying the code.
> new-mainnet-status is our divine revelation. The only modifications allowed are those needed to plug into debug commands like heal anchor and heal synth."

AIs that write divine code are adhering to the divine mantra. The implementation must exactly match the behavior of mainnet-status, with no deviations or optimizations.

## Overview

This document outlines the plan to replace the network initiation code in the heal anchor command with the implementation from new-mainnet-status. The new-mainnet-status tool has been proven to correctly discover all validators and check API V3 connectivity exactly like mainnet-status.

## Implementation Status

We have successfully implemented a new version of the network status tool (new-mainnet-status) that exactly matches the behavior of the original mainnet-status tool. This implementation:

1. ✅ Discovers all validators in the network using the same peer discovery process
2. ✅ Correctly checks API V3 connectivity for all nodes
3. ✅ Reports heights for all partitions across all validators
4. ✅ Identifies zombie nodes (BVNs significantly behind the Directory)
5. ✅ Reports the software version running on each node

## Next Steps

The next step is to replace the network initiation code in the heal anchor command with our proven implementation from new-mainnet-status. This will ensure that the heal anchor command has access to the complete and accurate network information it needs to function correctly.

## Implementation Details

The new-mainnet-status implementation is located at:
- `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/debug/docs2/debugging/new-mainnet-status/`

Key files:
- `node_info.go`: Contains the network initialization and validator discovery code
- `network_report.go`: Contains the code for generating the network report

Documentation:
- `README.md`: Contains a high-level overview of the tool and its implementation
- `TECHNICAL_DETAILS.md`: Contains detailed technical information about the implementation

## Important Notes

When implementing this replacement, it's crucial to:

1. **Maintain Exact Behavior**: The implementation must exactly match the behavior of mainnet-status
2. **Handle All Edge Cases**: The implementation must handle all edge cases correctly
3. **Ensure Proper Error Handling**: The implementation must handle errors gracefully
4. **Set Appropriate Timeouts**: All network operations must have appropriate timeouts

# Performance Test Results -- RC v1.5.1-breaking

**Date**: 2026-04-15T09:29:34Z
**Methodology**: Continuous ramp 1K->25K TPS (+2K every 30s), cutoff at 5.0% errors
**Duration per topology**: 600s
**Topologies**: {2,3,4} validators x {1,2,3} BVNs (every node runs DN+BVN)

## Results

| Topology | Nodes | Peak TPS | Avg TPS | Submitted | Failed |
|----------|-------|----------|---------|-----------|--------|
| 2v x 1b | 2 | 3761 | 2237.02 | 100667 | 25152 |
| 2v x 2b | 4 | 5573 | 3198.27 | 191901 | 10115 |
| 2v x 3b | 6 | 18835 | 12974.70 | 7784843 | 56999 |
| 3v x 1b | 3 | 4591 | 2518.61 | 151118 | 32372 |
| 3v x 2b | 6 | 19243 | 12426.89 | 7456150 | 87384 |
| 3v x 3b | 9 | 20309 | 16252.96 | 9751814 | 22 |
| 4v x 1b | 4 | 5363 | 3200.07 | 192007 | 10644 |
| 4v x 2b | 8 | 20513 | 15617.67 | 9370621 | 25 |
| 4v x 3b | 12 | 20663 | 17446.91 | 10468167 | 23 |

## Detailed Output

See individual `*-output.txt` files in this directory.

## Test Log

See `/tmp/loadtest-workspace/log.jsonl` for the full event log across all topologies.

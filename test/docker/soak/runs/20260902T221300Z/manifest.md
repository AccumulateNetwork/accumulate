# Soak run 20260902T221300Z

**Purpose:** validate #4189 durable unhashed staging + #4201 heal cadence: staging left PartitionSyntheticLedger for durable, unhashed, snapshotted account records; MaxPendingSequenced (4096) and its silent refusal deleted; healing asks staging and scans peers not ledger entries; heal cadence every 4 blocks with 2 selected senders for PULLS only (anchor push stays every validator); jitter, back-off windows and circuit breakers deleted. Same 12h/500tps as 20260902T132651Z, which livelocked with recv-deliv pinned at exactly 4096. THE TEST: BVN2->BVN1 must not sit at 4096 undelivered, and heals must not climb into six figures against a stalled stream.

| field | value |
|---|---|
| started (UTC) | 2026-09-02T22:13:00Z |
| commit | `91b5f38ecab949bf25531e76ec6a96c059f19bc6` |
| describe | `10k-tps-653-g91b5f38ec` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:1aa1bee25dcba256e1632252fcaa9bba01ea99a63f83ee6eeed0f6dc677a9d2a` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | leveldb |
| memory budget | mem_limit 1536m, GOMEMLIMIT 1200MiB |

Config as run is frozen in `config/`. Results appended below on exit.

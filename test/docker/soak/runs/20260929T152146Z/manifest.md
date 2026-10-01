# Soak run 20260929T152146Z

**Purpose:** #4466 docker proof: fresh network on lead 03945b6eb; after steady load, one deliberate whole-network restart; every validator must resume and anchors reach quorum

| field | value |
|---|---|
| started (UTC) | 2026-09-29T15:21:46Z |
| commit | `03945b6ebffcc71035b330b0f5c767c4c1e32c2a` |
| describe | `backup/dta-e11-before-lead-sync-437-g03945b6eb-dirty` |
| branch | `issue-4205-lead` |
| uncommitted files | 4284: 1 tracked (in `config/uncommitted.patch`), 4283 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:2c0af9ae0beb566af0d0682178b31238c7822a885eb79ca1b26f34e823529bbe` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:7f8705126b178fad6af8daa017cb518846c437f73b5993c33f5c88a75df2f46a` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:01e36c3d4f21c29c3d9a99950623702d32e720d5bfdf680f81337663b7399378` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:0224967b1faa542c9e834be5de33ce70121828207935263381da46f52b445238` (after the build) |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | none (CHAOS=off) |
| topology | 3 BVNs, 12 validators + 1 follower (acc-bvn3-fol1, partitions Directory BVN3) + bootstrap |
| follower key | inactive in the definition; active validators per partition: BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | off |
| target duration | 72h |
| target TPS | 1 |
| storage | BlockchainDB (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

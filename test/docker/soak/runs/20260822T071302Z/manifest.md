# Soak run 20260822T071302Z

**Purpose:** VERIFICATION run for #4132/#4133 on 6dabc1c14+a36be5e32: signer-affinity worker routing (hash+mask), received batches spread by digest instead of piling into worker 0, num-workers 100 -> 64. THE TEST: bootstrap phase A should now report 100/100 sub-treasuries funded instead of 0/100, and the treasury key should produce no timestamp-not-increasing rejections. If it does, DN 553 is gone too.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T07:13:02Z |
| commit | `a36be5e32698491e9773186dfaf699552bad74db` |
| describe | `10k-tps-432-ga36be5e32-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 5 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 20m |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

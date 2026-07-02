# Prompt: follower-to-validator failover test (#4050)

You are implementing and running a docker-based acceptance test that validates
**moving a validator's identity to a former follower node** — the
disaster-recovery / infra-migration path (the HorizonIQ move). A validator is
defined by its `priv_validator_key.json` (the consensus signing key that appears
in the on-chain validator set). Copying that key to another node makes *that*
node the validator, with **no validator-set change**.

Work on branch `4050-follower-to-validator-failover` (off `v1.4.3.1`).

## Deliverables

Under `test/docker/`:
1. `failover-network.yml` — a `NetworkInit` config: **one BVN, 5 dual nodes** — node-1 a validator, node-2..5 followers.
2. `docker-compose.failover.yml` — an `init` service + 5 `run-dual` node services on a shared volume (model on `docker-compose.net4049.yml`; reuse `Dockerfile.net4049`).
3. `failover-test.sh` — the acceptance test that drives and checks the scenario below.

## Topology

Single BVN, 5 dual nodes (each runs a DN validator/follower + a BVN
validator/follower):
- **node-1**: `dnnType: validator`, `bvnnType: validator`
- **node-2 … node-5**: `dnnType: follower`, `bvnnType: follower`

Static peering via docker DNS hostnames (e.g. `accfo-node-1..5`), same pattern as
`network-4049.yml`. `init network <cfg>` generates per-node dirs
(`node-1/…`) with `accumulate.toml` + `directory-genesis.snap` + `bvn*-genesis.snap`;
nodes run via `run-dual /data/<node>/dnn /data/<node>/bvnn`.

## Scenario the test must perform

1. **Deploy** the network; wait until the BVN produces blocks (height advances
   past genesis on node-1's API).
2. **Stop the validator** (node-1). Confirm the BVN **halts** — block height stops
   advancing (its sole validator is down). This is expected.
3. **Stop one follower** (node-2).
4. **Promote node-2 to validator** — copy node-1's validator identity onto node-2:
   - `node-1/dnn/config/priv_validator_key.json` → `node-2/dnn/config/priv_validator_key.json`
   - `node-1/bvnn/config/priv_validator_key.json` → `node-2/bvnn/config/priv_validator_key.json`
   - Also reset node-2's `priv_validator_state.json` for both partitions to a
     clean state (height 0) so it does not conflict with node-1's last-signed
     state. The validator set is unchanged (same pubkey).
   - Leave node-1 **stopped** for the remainder of the test.
5. **Restart the network**: node-2 (now the validator) + node-3..5 (followers).
6. Confirm **node-2 now signs as the validator** and the BVN **resumes producing
   blocks** under the same validator identity.

## PASS / FAIL

- **PASS**: after the promotion + restart, the BVN's block height advances again,
  the active validator pubkey equals node-1's original validator pubkey, and no
  on-chain validator-set change occurred.
- **FAIL**: the BVN never resumes producing blocks, or the validator identity
  differs.

## How to verify (no guessing)

- Block progress: query a running node's API (`:26660` → mapped host port) for the
  partition's last block height before/after each step.
- Validator identity: compare the promoted node's `priv_validator_key.json` pubkey
  to node-1's original (they must be equal) and confirm it matches the on-chain
  validator set for the BVN.
- Halt during outage: height must be flat while node-1 is down and node-2 is not
  yet promoted/running.

## Safety (critical)

**Never run node-1 and node-2 with the same validator key at the same time** —
that is a double-sign and would be slashed on a real network. node-1 must be fully
stopped (step 2) before node-2 starts with the key (step 5). The test must enforce
this ordering and must not restart node-1.

## Notes

- Reuse `Dockerfile.net4049` (only builds `accumulated`; the root Dockerfile's tool
  installs are heavier and unnecessary here).
- Ports: map each node's `26660` to a distinct host port for API queries.
- Redirect verbose output to log files; poll with `tail`/short loops, don't stream.
- Related: #4049 (boot-without-snapshot, exercised on the restarts) and the
  validator-key migration for the HorizonIQ move.

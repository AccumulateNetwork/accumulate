# Cyclops Deployment: Deploy Phase

This document details the steps for deploying prepared artifacts to the validator node. These steps assume the Prep phase has been completed and all artifacts are present in `~/accumulate-network/artifacts` on the build/ops machine.

## Steps

1. **Clean Previous Deployment**
   - Remove any previous deployment at `/tmp/cyclops/` on the validator node.

2. **Create Artifacts Directory**
   - Create the directory `/tmp/cyclops/artifacts` on the validator node.

3. **Copy Artifacts**
   - Copy the following files from `~/accumulate-network/artifacts` to `/tmp/cyclops/artifacts`:
     - `cyclops-genesis.snap`
     - `priv_validator_key_dn.json`
     - `priv_validator_key_bvn0.json`
     - `cyclops_network.json`
     - `consensus_dn.json`
     - `consensus_bvn0.json`

4. **Node Configuration Construction**
   - Use the artifacts in `/tmp/cyclops/artifacts` to construct the node configuration as required by the deployment process.

5. **Initialize Node**
   - Run `accumulated init node` (or the appropriate command) using the artifacts from `/tmp/cyclops`.

6. **Verify Deployment**
   - Ensure TOML configuration files are generated and placed correctly.
   - Ensure `priv_validator_key.json` files are present and correct for both partitions.
   - Ensure partition snapshots are present and correct.

---

**Result:**
The validator node at `/tmp/cyclops` is configured with all required artifacts and ready for launch.

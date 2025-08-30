# Directory Network Height Issue - Summary

## The Problem
The public Accumulate mainnet APIs do not expose the real-time Directory Network (DN) height, even though the DN is actively producing blocks on the validator nodes.

## Evidence Found

### 1. DN IS Active (Proven)
- **lastBlockTime** from V2 API `query-directory` updates every ~3 seconds
- You confirmed the DN height IS changing when viewing it on AWS validator nodes via accman
- The DN is definitely producing blocks

### 2. Public APIs Return Cached/Static Values
- **V3 API** (`network-status`): Returns static `directoryHeight: 2460315`
- **V2 API** (`query` on anchors): Returns static `minorBlockSequenceNumber: 4857`
- These values do NOT update even though the DN is active

### 3. No Public Endpoint Found
After extensive testing, NO public endpoint was found that exposes the real-time DN height:
- Tested: mainnet.accumulatenetwork.io (V2 and V3)
- Tested: apollo-mainnet.accumulate.defidevs.io (V2 and V3)
- Tested: Various ports (16692, 16695, 16696, 8080, etc.)
- Tested: All available query methods and parameters

## Root Cause
The public API infrastructure appears to be disconnected from or not properly syncing with the actual DN validator nodes. The APIs are serving cached data rather than real-time blockchain state.

## Solutions

### 1. Infrastructure Fix (Required)
The public API servers need to be fixed to properly connect to and sync with the DN validators. This is an operational issue that needs to be addressed by the Accumulate team.

### 2. Direct Validator Access
The real DN height can only be obtained by:
- Connecting directly to validator nodes (like your AWS instances)
- Running your own validator or follower node
- Using internal/private APIs that have direct access to the validators

### 3. Current Workaround
For now, the network monitor shows:
- The cached DN height from the public API (with a warning)
- The live Cyclops BVN height (which IS updating)
- The lastBlockTime to prove DN activity

## Recommendation
Contact the Accumulate team to report that the public API infrastructure is not exposing real-time DN height data. This is a critical monitoring metric that should be available through the public APIs.
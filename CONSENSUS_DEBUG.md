# Consensus Debugging: TestIntegration_ThreeNodes Timeout

## Problem Statement

3-node consensus cluster fails to form certificates in round 1.

**Symptoms**:
- Each node creates exactly 1 header (round 1)
- Headers are successfully gossipped between all nodes
- NO votes are being aggregated into certificates
- Nodes timeout waiting for round 2
- Test output shows: `headersCreated=1 certificatesCreated=0`

## Expected Behavior

1. **Genesis (round 0)**: 3 certificates created (one per validator) via `InsertGenesisForAll`
2. **Round 1**:
   - Each of 3 nodes creates 1 header referencing round 0 certificates as parents
   - Each node receives headers from the other 2 nodes via gossip
   - Each node votes on received headers (should send 2 votes per node = 6 votes total per node)
   - Votes are aggregated: each header collects votes from other validators
   - When 2+ validators' votes are received for a header → quorum reached (2f+1 for 3 nodes)
   - Certificates are created from quorum votes
   - Round advances to round 2

## Actual Behavior

1. **Genesis**: ✅ Works (InsertGenesisForAll successfully creates round 0 certs)
2. **Round 1 Header Creation**: ✅ Works (headersCreated=1 per node)
3. **Header Gossip**: ✅ Works (nodes log "Received header via gossip")
4. **Vote Generation**: ❌ FAILS or not logged
5. **Vote Aggregation**: ❌ No votes collected (certificatesCreated=0)
6. **Certificate Formation**: ❌ Blocked due to no quorum

## Code Flow Investigation

### Where Certificates Should Be Created
- **Entry point**: `OnVoteReceived()` in `pkg/consensus/primary/vote_handler.go:18`
- **Processing**:
  1. Validate vote (committee membership, signature)
  2. Check vote references our created header
  3. Add vote to `pendingVotes[headerDigest]`
  4. Call `tryCreateCertificateLocked(headerDigest)`
  5. If quorum reached → create certificate

### Where Votes Should Be Sent
- **Entry point**: `OnHeaderReceived()` in `pkg/consensus/primary/vote_handler.go:235`
- **Processing**:
  1. Validate header signature
  2. Check author is in committee
  3. Don't vote on own headers
  4. Verify header epoch/round matches current state
  5. Check parent certificates exist in DAG
  6. Create vote → sign → send via gossip

### Potential Failure Points

**Point 1: Header validation fails** (line 235-295)
- Signature verification fails (unlikely, signed by real validators)
- Author not in committee (unlikely, all nodes know the committee)
- Epoch mismatch (possible if epoch handling is broken)
- Round out of range (possible if currentRound tracking is broken)

**Point 2: Parent certificate check fails** (line 306-313)
```go
for _, parentDigest := range header.Parents {
    if p.dag.GetByDigest(parentDigest) == nil {
        slog.Debug("Missing parent for header",...)
        return // missing parent, can't vote
    }
}
```
- If headers are created without proper parent references
- If parents are created but not in DAG when header received
- If DAG lookup is broken

**Point 3: Vote creation/sending fails** (line 316-357)
- Signing fails (unlikely)
- Gossip broadcast fails (possible)
- Votes never reach the creator's node

**Point 4: Votes don't reach vote handler** (line 18 not called)
- Gossip not delivering votes
- Node not subscribed to vote channel
- Votes filtered/dropped somewhere

**Point 5: Vote aggregation blocked** (line 112-143)
- Votes reach handler but don't aggregate
- Quorum calculation broken
- Committee membership check too strict

## Testing Strategy

### Test 1: Verify Genesis Setup
Add logging to see:
- How many genesis certificates are in each node's DAG after InsertGenesisForAll
- Verify all 3 nodes have all 3 round-0 certs

### Test 2: Trace Header Creation
Add logging to see:
- What parents are put in round 1 headers
- Are parents valid round-0 certificates?
- Are headers properly signed?

### Test 3: Trace Vote Generation
Add logging in `OnHeaderReceived`:
- Log each received header
- Log outcome of each validation check
- Log if we're sending a vote or why we're not
- Log the vote that we're sending

### Test 4: Trace Vote Reception
Add logging in `OnVoteReceived`:
- Log each vote received
- Log total votes accumulated for each header
- Log when tryCreateCertificateLocked is called
- Log the quorum result (stake vs threshold)

### Test 5: Verify Gossip
- Is gossip layer delivering votes like it delivers headers?
- Are votes reaching all nodes?

## Key Files to Investigate

1. `pkg/consensus/primary/primary.go` - Header creation (CreateHeader method)
2. `pkg/consensus/primary/vote_handler.go` - Vote generation and aggregation
3. `pkg/consensus/types/header.go` - Header creation and parent handling
4. `pkg/consensus/gossip/gossip.go` - Message distribution
5. `pkg/consensus/types/dag.go` - DAG operations (certificate lookup)
6. `pkg/consensus/primary/committee.go` - Quorum calculation

## Hypothesis

Most likely:
- **Hypothesis A**: Headers don't include proper parent references (Parents field is empty or wrong)
  - Result: OnHeaderReceived fails the parent check and doesn't send vote
  - Fix: Ensure headers reference genesis certificates

- **Hypothesis B**: DAG lookup for parents fails
  - Result: Even with correct parent references, `dag.GetByDigest()` returns nil
  - Fix: Verify DAG is shared between nodes or synced properly

- **Hypothesis C**: Vote gossip is broken (votes don't reach other nodes)
  - Result: Nodes send votes but only to themselves, quorum never reached
  - Fix: Check gossip channel subscription and message delivery

- **Hypothesis D**: Quorum calculation is wrong for 3-node cluster
  - Result: Even with votes, committee.HasQuorum() returns false
  - Fix: Verify 2f+1 calculation (f=1 for 3 nodes, quorum=2)

## Next Actions

1. Add comprehensive logging to vote handler
2. Run test with logging enabled
3. Compare vote creation vs vote reception
4. Check DAG state at each node
5. Verify quorum calculation for 3-node setup


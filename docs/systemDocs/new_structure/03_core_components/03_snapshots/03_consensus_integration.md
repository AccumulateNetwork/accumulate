---
title: CometBFT Integration with Accumulate Snapshots
description: An analysis of how Accumulate's snapshot system integrates with CometBFT for state synchronization
tags: [accumulate, snapshots, cometbft, state-sync, integration]
created: 2025-05-16
version: 1.0
---

# CometBFT Integration with Accumulate Snapshots

## 1. Introduction

This document analyzes how Accumulate's snapshot system integrates with CometBFT (formerly Tendermint) for state synchronization. CometBFT provides a state sync protocol that allows nodes to quickly bootstrap by downloading and applying a snapshot of the application state rather than replaying the entire blockchain history. This integration is crucial for efficient node operation in the Accumulate network.

## 2. CometBFT State Sync Overview

### 2.1 State Sync Protocol

CometBFT's state sync protocol operates through four ABCI (Application Blockchain Interface) methods:

1. **ListSnapshots**: Returns a list of available snapshots
2. **LoadSnapshotChunk**: Loads a chunk of a snapshot for distribution
3. **OfferSnapshot**: Receives a snapshot offer from another node
4. **ApplySnapshotChunk**: Applies a chunk of a received snapshot

These methods form a protocol that allows nodes to:
- Discover available snapshots from peers
- Select an appropriate snapshot
- Download the snapshot in chunks
- Apply the snapshot to restore application state

### 2.2 CometBFT Requirements

For state sync to work properly, CometBFT requires:

1. A trusted block height and corresponding application hash
2. At least one snapshot at or before the trusted height
3. Proper implementation of the four ABCI methods
4. Consistent application state after restoration

## 3. Accumulate's ABCI Implementation

### 3.1 ListSnapshots

Accumulate implements the `ListSnapshots` method to provide a list of available snapshots:

```go
func (app *Accumulator) ListSnapshots(_ context.Context, req *abci.RequestListSnapshots) (*abci.ResponseListSnapshots, error) {
    info, err := ListSnapshots(config.MakeAbsolute(app.RootDir, app.Snapshots.Directory))
    if err != nil {
        return nil, err
    }

    resp := new(abci.ResponseListSnapshots)
    resp.Snapshots = make([]*abci.Snapshot, 0, len(info))
    for _, info := range info {
        b, err := info.fileMd.MarshalBinary()
        if err != nil {
            return nil, err
        }

        resp.Snapshots = append(resp.Snapshots, &abci.Snapshot{
            Height:   info.Height(),
            Format:   uint32(info.Version()),
            Chunks:   uint32(len(info.fileMd.Chunks)),
            Hash:     info.fileHash[:],
            Metadata: b,
        })
    }
    return resp, nil
}
```

This method:
1. Scans the snapshot directory for available snapshots
2. Reads metadata from each snapshot
3. Converts the metadata to CometBFT's format
4. Returns the list of available snapshots

### 3.2 LoadSnapshotChunk

Accumulate implements the `LoadSnapshotChunk` method to provide chunks of a snapshot:

```go
func (app *Accumulator) LoadSnapshotChunk(_ context.Context, req *abci.RequestLoadSnapshotChunk) (*abci.ResponseLoadSnapshotChunk, error) {
    snapDir := config.MakeAbsolute(app.RootDir, app.Snapshots.Directory)
    f, err := os.Open(filepath.Join(snapDir, fmt.Sprintf(core.SnapshotMajorFormat, req.Height)))
    if err != nil {
        return nil, err
    }
    defer f.Close()

    _, err = f.Seek(int64(req.Chunk)*chunkSize, io.SeekStart)
    if err != nil {
        return nil, err
    }

    var buf [chunkSize]byte
    _, err = io.ReadFull(f, buf[:])
    if err != nil {
        return nil, err
    }

    return &abci.ResponseLoadSnapshotChunk{Chunk: buf[:]}, nil
}
```

This method:
1. Opens the snapshot file for the requested height
2. Seeks to the position of the requested chunk
3. Reads the chunk data
4. Returns the chunk data

### 3.3 OfferSnapshot

Accumulate implements the `OfferSnapshot` method to receive snapshot offers:

```go
func (app *Accumulator) OfferSnapshot(_ context.Context, req *abci.RequestOfferSnapshot) (*abci.ResponseOfferSnapshot, error) {
    if req.Snapshot == nil {
        return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}, nil
    }
    if req.Snapshot.Format != sv2.Version2 {
        return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT_FORMAT}, nil
    }

    ok, err := app.snapshots.Start(req)
    if !ok {
        return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}, nil
    }
    if err != nil {
        return nil, err
    }
    return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_ACCEPT}, nil
}
```

This method:
1. Validates the snapshot offer
2. Checks if the snapshot format is supported
3. Initializes the snapshot download process
4. Returns an acceptance or rejection response

### 3.4 ApplySnapshotChunk

Accumulate implements the `ApplySnapshotChunk` method to apply received chunks:

```go
func (app *Accumulator) ApplySnapshotChunk(_ context.Context, req *abci.RequestApplySnapshotChunk) (*abci.ResponseApplySnapshotChunk, error) {
    err := app.snapshots.Apply(int(req.Index), req.Chunk)
    switch {
    case errors.Is(err, torrent.ErrBadHash):
        return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_RETRY}, nil
    case err != nil:
        return nil, err
    case !app.snapshots.Done():
        return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ACCEPT}, nil
    }

    // All chunks received, restore the snapshot
    buf := new(bytes.Buffer)
    bptHash := app.snapshots.request.AppHash

    e1 := app.snapshots.WriteTo(buf)
    e2 := app.snapshots.Reset() // Reset regardless of success
    if e1 != nil {
        return nil, e1
    }
    if e2 != nil {
        return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_RETRY_SNAPSHOT}, nil
    }

    // Restore the snapshot
    // ...

    return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ACCEPT}, nil
}
```

This method:
1. Applies the received chunk to the download manager
2. Checks if all chunks have been received
3. If all chunks are received, assembles and verifies the snapshot
4. Restores the application state from the snapshot
5. Returns an acceptance, retry, or rejection response

## 4. Snapshot Manager Implementation

### 4.1 Download Manager

Accumulate uses a torrent-like download manager to handle snapshot chunks:

```go
type snapshotManager struct {
    active  *torrent.DownloadJob
    request *abci.RequestOfferSnapshot
}
```

This manager:
1. Tracks the download progress
2. Verifies chunk hashes
3. Assembles chunks into a complete snapshot
4. Verifies the complete snapshot hash

### 4.2 Chunk Processing

Chunks are processed using a fixed size (10MB) to balance network efficiency and memory usage:

```go
const chunkSize = 10 << 20 // 10MB
```

The chunk processing workflow:
1. Each chunk is received via `ApplySnapshotChunk`
2. The chunk is added to the download job
3. The chunk hash is verified
4. When all chunks are received, the complete snapshot is assembled
5. The complete snapshot hash is verified against the expected hash

### 4.3 Snapshot Verification

Before applying a snapshot, Accumulate verifies:
1. The snapshot format is supported
2. The snapshot hash matches the expected hash
3. The snapshot can be successfully parsed
4. The BPT root hash matches the expected hash

## 5. Integration Challenges and Solutions

### 5.1 Version Compatibility

**Challenge**: Ensuring compatibility between different snapshot versions and CometBFT.

**Solution**: Accumulate supports multiple snapshot versions and validates the version during the offer phase:

```go
if req.Snapshot.Format != sv2.Version2 {
    return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT_FORMAT}, nil
}
```

### 5.2 Chunk Management

**Challenge**: Efficiently managing and verifying chunks of large snapshots.

**Solution**: Accumulate uses a torrent-like download manager with hash verification:

```go
func (m *snapshotManager) Apply(index int, chunk []byte) error {
    if m.active == nil {
        return errors.BadRequest.With("not started")
    }

    return m.active.RecordChunk(index, chunk)
}
```

### 5.3 State Consistency

**Challenge**: Ensuring application state is consistent after restoration.

**Solution**: Accumulate verifies the BPT root hash after restoration:

```go
// Verify the BPT hash
gotHash, err := db.GetBptRootHash()
if err != nil {
    return nil, errors.UnknownError.WithFormat("get BPT root hash: %w", err)
}
if !bytes.Equal(gotHash, bptHash) {
    return nil, errors.UnknownError.WithFormat("BPT hash mismatch: want %x, got %x", bptHash, gotHash)
}
```

### 5.4 Consensus State Integration

**Challenge**: Integrating application state with consensus state.

**Solution**: Accumulate stores consensus state in a separate section in Version 2 snapshots:

```go
func (c *snapshotCollector) collectConsensusDoc(w *sv2.Writer, minorBlock uint64) error {
    // Get consensus block
    // Marshal consensus doc
    // Write to consensus section
}
```

## 6. Performance Optimizations

### 6.1 Chunk Size Optimization

Accumulate uses a 10MB chunk size, which balances:
- Network efficiency (fewer round trips)
- Memory usage (manageable chunk size)
- Retry efficiency (smaller chunks are easier to retry)

### 6.2 Parallel Chunk Downloads

CometBFT's state sync protocol allows for parallel chunk downloads, which Accumulate leverages for faster synchronization.

### 6.3 Snapshot Scheduling

Accumulate optimizes snapshot creation through scheduling:

```go
func (c *snapshotCollector) isTimeForSnapshot(blockTime time.Time) bool {
    // Check for manual trigger
    // Check schedule
    // Check existing snapshots
    // Determine if it's time for a new snapshot
}
```

## 7. User Experience

### 7.1 Configuration

Users can configure state sync in their CometBFT configuration:

```toml
[statesync]
enable = true
rpc_servers = "node1.example.com:26657,node2.example.com:26657"
trust_height = 1000000
trust_hash = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
```

### 7.2 Monitoring

Users can monitor state sync progress through CometBFT logs and Accumulate's API:

```bash
curl -X GET "http://localhost:26660/v3/snapshots"
```

### 7.3 Troubleshooting

Common issues and solutions:
1. **No snapshots available**: Ensure snapshots are being created on seed nodes
2. **Snapshot download fails**: Check network connectivity and retry
3. **Snapshot application fails**: Check disk space and verify trust hash

## 8. Future Improvements

### 8.1 Enhanced Metadata

Adding more detailed metadata to snapshots would improve selection and verification:
- Network parameters
- Validator set information
- Protocol version information

### 8.2 Differential Snapshots

Implementing differential snapshots would reduce storage and transfer requirements:
- Base snapshots at major heights
- Differential snapshots for intermediate heights
- Reconstruction of full state from base + differentials

### 8.3 Snapshot Pruning

Implementing more sophisticated pruning strategies would optimize resource usage:
- Keep more recent snapshots
- Keep snapshots at significant heights
- Prune based on network conditions

## 9. Conclusion

Accumulate's integration with CometBFT's state sync protocol provides a robust mechanism for efficient node bootstrapping. The implementation balances performance, security, and resource usage through careful design and optimization. This integration is crucial for the scalability and reliability of the Accumulate network, allowing new nodes to join quickly and existing nodes to recover efficiently.

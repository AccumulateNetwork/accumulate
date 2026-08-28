// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Protocol ID suffixes for request/response protocols. The full ID is
// PARTITION-SCOPED — "/acc/consensus/<partition>/batch/1.0.0" — because a
// production validator runs several partitions' Nodes on ONE shared libp2p
// host, and libp2p SetStreamHandler on a shared ID overwrites: the
// last-started partition's handler (backed by ITS batch store) served every
// fetch on the host, so peer batch-fetch was structurally dead for all other
// partitions — peerAsks in the hundreds of thousands, peerHits=0 (#4159,
// #4125's signature). Scoping by partition gives each Node its own handler.
const (
	protocolBatchFetchSuffix = "/batch/1.0.0"
	protocolCertFetchSuffix  = "/cert/1.0.0"
	protocolDAGSyncSuffix    = "/sync/1.0.0"
)

// Default timeouts for protocol operations.
const (
	DefaultReadTimeout  = 30 * time.Second
	DefaultWriteTimeout = 30 * time.Second
)

// Maximum message sizes to prevent DoS.
const (
	MaxBatchSize        = 10 * 1024 * 1024  // 10 MB
	MaxCertificateSize  = 1 * 1024 * 1024   // 1 MB
	MaxSyncResponseSize = 100 * 1024 * 1024 // 100 MB
)

// BatchStore provides access to stored batches.
type BatchStore interface {
	// GetBatch retrieves a batch by its digest.
	// Returns nil, nil if the batch is not found.
	GetBatch(digest types.BatchDigest) (*types.Batch, error)

	// StoreBatch stores a batch.
	StoreBatch(batch *types.Batch) error
}

// DAGStore provides access to the DAG of certificates.
type DAGStore interface {
	// GetCertificate retrieves a certificate by round and author.
	// Returns nil, nil if the certificate is not found.
	GetCertificate(round types.Round, author []byte) (*types.Certificate, error)

	// GetRound retrieves all certificates for a given round.
	// Returns an empty slice if no certificates exist for the round.
	GetRound(round types.Round) ([]*types.Certificate, error)
}

// BatchRequest is a request to fetch a batch by digest.
type BatchRequest struct {
	Digest types.BatchDigest
}

// Marshal serializes the request.
func (r *BatchRequest) Marshal() []byte {
	return r.Digest[:]
}

// UnmarshalBatchRequest deserializes a batch request.
func UnmarshalBatchRequest(data []byte) (*BatchRequest, error) {
	if len(data) != 32 {
		return nil, errors.New("invalid batch request size")
	}
	req := &BatchRequest{}
	copy(req.Digest[:], data)
	return req, nil
}

// CertRequest is a request to fetch a certificate by round and author.
type CertRequest struct {
	Round  types.Round
	Author []byte // public key (32 bytes)
}

// Marshal serializes the request.
func (r *CertRequest) Marshal() []byte {
	data := make([]byte, 8+len(r.Author))
	binary.BigEndian.PutUint64(data[0:8], uint64(r.Round))
	copy(data[8:], r.Author)
	return data
}

// UnmarshalCertRequest deserializes a certificate request.
func UnmarshalCertRequest(data []byte) (*CertRequest, error) {
	if len(data) < 8 {
		return nil, errors.New("certificate request too short")
	}
	req := &CertRequest{
		Round:  types.Round(binary.BigEndian.Uint64(data[0:8])),
		Author: make([]byte, len(data)-8),
	}
	copy(req.Author, data[8:])
	return req, nil
}

// DAGSyncRequest is a request to sync certificates for a range of rounds.
type DAGSyncRequest struct {
	FromRound types.Round // inclusive
	ToRound   types.Round // inclusive
}

// Marshal serializes the request.
func (r *DAGSyncRequest) Marshal() []byte {
	data := make([]byte, 16)
	binary.BigEndian.PutUint64(data[0:8], uint64(r.FromRound))
	binary.BigEndian.PutUint64(data[8:16], uint64(r.ToRound))
	return data
}

// UnmarshalDAGSyncRequest deserializes a DAG sync request.
func UnmarshalDAGSyncRequest(data []byte) (*DAGSyncRequest, error) {
	if len(data) != 16 {
		return nil, errors.New("invalid DAG sync request size")
	}
	return &DAGSyncRequest{
		FromRound: types.Round(binary.BigEndian.Uint64(data[0:8])),
		ToRound:   types.Round(binary.BigEndian.Uint64(data[8:16])),
	}, nil
}

// ProtocolHandler manages request/response protocols for consensus data.
type ProtocolHandler struct {
	host       host.Host
	partition  string
	batchStore BatchStore
	dagStore   DAGStore
}

// NewProtocolHandler creates a new ProtocolHandler for one partition's Node.
// The partition scopes the wire protocol IDs — see the suffix constants.
func NewProtocolHandler(h host.Host, partition string, bs BatchStore, ds DAGStore) (*ProtocolHandler, error) {
	if h == nil {
		return nil, errors.New("host is nil")
	}
	if partition == "" {
		return nil, errors.New("partition is empty")
	}
	return &ProtocolHandler{
		host:       h,
		partition:  partition,
		batchStore: bs,
		dagStore:   ds,
	}, nil
}

// pid returns the partition-scoped protocol ID for a suffix. Requester and
// responder both derive it from their own Node's partition, so they agree —
// and different partitions on one host no longer clobber each other's
// handlers.
func (p *ProtocolHandler) pid(suffix string) protocol.ID {
	return protocol.ID("/acc/consensus/" + p.partition + suffix)
}

// RegisterHandlers registers stream handlers for all protocols.
// The handler will respond to incoming requests using the configured stores.
func (p *ProtocolHandler) RegisterHandlers() error {
	if p.batchStore != nil {
		p.host.SetStreamHandler(p.pid(protocolBatchFetchSuffix), p.handleBatchFetch)
	}
	if p.dagStore != nil {
		p.host.SetStreamHandler(p.pid(protocolCertFetchSuffix), p.handleCertFetch)
		p.host.SetStreamHandler(p.pid(protocolDAGSyncSuffix), p.handleDAGSync)
	}
	return nil
}

// UnregisterHandlers removes all protocol handlers.
func (p *ProtocolHandler) UnregisterHandlers() {
	p.host.RemoveStreamHandler(p.pid(protocolBatchFetchSuffix))
	p.host.RemoveStreamHandler(p.pid(protocolCertFetchSuffix))
	p.host.RemoveStreamHandler(p.pid(protocolDAGSyncSuffix))
}

// FetchBatch requests a batch from a peer by digest.
func (p *ProtocolHandler) FetchBatch(ctx context.Context, peerID peer.ID, digest types.BatchDigest) (*types.Batch, error) {
	stream, err := p.host.NewStream(ctx, peerID, p.pid(protocolBatchFetchSuffix))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// Send request
	req := &BatchRequest{Digest: digest}
	if err := writeMessage(stream, req.Marshal(), DefaultWriteTimeout); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Close write side to signal we're done sending
	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read response
	data, err := readMessage(stream, MaxBatchSize, DefaultReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Empty response means not found
	if len(data) == 0 {
		return nil, nil
	}

	batch, err := types.UnmarshalBatch(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal batch: %w", err)
	}

	return batch, nil
}

// FetchCertificate requests a certificate from a peer by round and author.
func (p *ProtocolHandler) FetchCertificate(ctx context.Context, peerID peer.ID, round types.Round, author []byte) (*types.Certificate, error) {
	stream, err := p.host.NewStream(ctx, peerID, p.pid(protocolCertFetchSuffix))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// Send request
	req := &CertRequest{Round: round, Author: author}
	if err := writeMessage(stream, req.Marshal(), DefaultWriteTimeout); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Close write side
	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read response
	data, err := readMessage(stream, MaxCertificateSize, DefaultReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Empty response means not found
	if len(data) == 0 {
		return nil, nil
	}

	cert, err := types.UnmarshalCertificate(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal certificate: %w", err)
	}

	return cert, nil
}

// SyncDAG requests all certificates for a range of rounds from a peer.
func (p *ProtocolHandler) SyncDAG(ctx context.Context, peerID peer.ID, fromRound, toRound types.Round) ([]*types.Certificate, error) {
	stream, err := p.host.NewStream(ctx, peerID, p.pid(protocolDAGSyncSuffix))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// Send request
	req := &DAGSyncRequest{FromRound: fromRound, ToRound: toRound}
	if err := writeMessage(stream, req.Marshal(), DefaultWriteTimeout); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Close write side
	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read response (format: count + serialized certs)
	data, err := readMessage(stream, MaxSyncResponseSize, DefaultReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Empty response means no certificates
	if len(data) == 0 {
		return nil, nil
	}

	return unmarshalCertificateList(data)
}

// handleBatchFetch handles incoming batch fetch requests.
func (p *ProtocolHandler) handleBatchFetch(stream network.Stream) {
	defer stream.Close()

	// Read request
	data, err := readMessage(stream, 32, DefaultReadTimeout)
	if err != nil {
		slog.Debug("Failed to read batch request", "error", err, "peer", stream.Conn().RemotePeer())
		return
	}

	req, err := UnmarshalBatchRequest(data)
	if err != nil {
		slog.Debug("Invalid batch request", "error", err, "peer", stream.Conn().RemotePeer())
		return
	}

	// Get batch from store
	batch, err := p.batchStore.GetBatch(req.Digest)
	if err != nil {
		slog.Debug("Failed to get batch", "error", err, "digest", req.Digest)
		return
	}

	// Send response (empty if not found)
	var response []byte
	if batch != nil {
		response, err = batch.Marshal()
		if err != nil {
			slog.Debug("Failed to marshal batch", "error", err)
			return
		}
	}

	if err := writeMessage(stream, response, DefaultWriteTimeout); err != nil {
		slog.Debug("Failed to send batch response", "error", err, "peer", stream.Conn().RemotePeer())
	}
}

// handleCertFetch handles incoming certificate fetch requests.
func (p *ProtocolHandler) handleCertFetch(stream network.Stream) {
	defer stream.Close()

	// Read request
	data, err := readMessage(stream, 256, DefaultReadTimeout)
	if err != nil {
		slog.Debug("Failed to read cert request", "error", err, "peer", stream.Conn().RemotePeer())
		return
	}

	req, err := UnmarshalCertRequest(data)
	if err != nil {
		slog.Debug("Invalid cert request", "error", err, "peer", stream.Conn().RemotePeer())
		return
	}

	// Get certificate from store
	cert, err := p.dagStore.GetCertificate(req.Round, req.Author)
	if err != nil {
		slog.Debug("Failed to get certificate", "error", err, "round", req.Round)
		return
	}

	// Send response (empty if not found)
	var response []byte
	if cert != nil {
		response, err = cert.Marshal()
		if err != nil {
			slog.Debug("Failed to marshal certificate", "error", err)
			return
		}
	}

	if err := writeMessage(stream, response, DefaultWriteTimeout); err != nil {
		slog.Debug("Failed to send cert response", "error", err, "peer", stream.Conn().RemotePeer())
	}
}

// handleDAGSync handles incoming DAG sync requests.
func (p *ProtocolHandler) handleDAGSync(stream network.Stream) {
	defer stream.Close()

	// Read request
	data, err := readMessage(stream, 16, DefaultReadTimeout)
	if err != nil {
		slog.Debug("Failed to read sync request", "error", err, "peer", stream.Conn().RemotePeer())
		return
	}

	req, err := UnmarshalDAGSyncRequest(data)
	if err != nil {
		slog.Debug("Invalid sync request", "error", err, "peer", stream.Conn().RemotePeer())
		return
	}

	// Validate range
	if req.ToRound < req.FromRound {
		slog.Debug("Invalid sync range", "from", req.FromRound, "to", req.ToRound)
		return
	}

	// Limit range to prevent excessive response sizes
	maxRounds := types.Round(1000)
	if req.ToRound-req.FromRound > maxRounds {
		req.ToRound = req.FromRound + maxRounds
	}

	// Collect certificates
	var certs []*types.Certificate
	for round := req.FromRound; round <= req.ToRound; round++ {
		roundCerts, err := p.dagStore.GetRound(round)
		if err != nil {
			slog.Debug("Failed to get round", "error", err, "round", round)
			continue
		}
		certs = append(certs, roundCerts...)
	}

	// Send response
	response, err := marshalCertificateList(certs)
	if err != nil {
		slog.Debug("Failed to marshal certificates", "error", err)
		return
	}

	if err := writeMessage(stream, response, DefaultWriteTimeout); err != nil {
		slog.Debug("Failed to send sync response", "error", err, "peer", stream.Conn().RemotePeer())
	}
}

// writeMessage writes a length-prefixed message to the stream.
func writeMessage(stream network.Stream, data []byte, timeout time.Duration) error {
	if err := stream.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}

	// Write length prefix (4 bytes)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := stream.Write(lenBuf); err != nil {
		return err
	}

	// Write data
	if len(data) > 0 {
		if _, err := stream.Write(data); err != nil {
			return err
		}
	}

	return nil
}

// readMessage reads a length-prefixed message from the stream.
func readMessage(stream network.Stream, maxSize int, timeout time.Duration) ([]byte, error) {
	if err := stream.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

	// Read length prefix
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if int(length) > maxSize {
		return nil, fmt.Errorf("message too large: %d > %d", length, maxSize)
	}

	// Handle empty messages
	if length == 0 {
		return nil, nil
	}

	// Read data
	data := make([]byte, length)
	if _, err := io.ReadFull(stream, data); err != nil {
		return nil, err
	}

	return data, nil
}

// marshalCertificateList serializes a list of certificates.
// Format: [count (4 bytes)] + for each cert: [len (4 bytes)][data]
func marshalCertificateList(certs []*types.Certificate) ([]byte, error) {
	// Calculate total size
	var totalSize int
	certData := make([][]byte, len(certs))
	for i, cert := range certs {
		data, err := cert.Marshal()
		if err != nil {
			return nil, err
		}
		certData[i] = data
		totalSize += 4 + len(data)
	}

	result := make([]byte, 4+totalSize)
	binary.BigEndian.PutUint32(result[0:4], uint32(len(certs)))

	offset := 4
	for _, data := range certData {
		binary.BigEndian.PutUint32(result[offset:], uint32(len(data)))
		offset += 4
		copy(result[offset:], data)
		offset += len(data)
	}

	return result, nil
}

// unmarshalCertificateList deserializes a list of certificates.
func unmarshalCertificateList(data []byte) ([]*types.Certificate, error) {
	if len(data) < 4 {
		return nil, errors.New("certificate list too short")
	}

	count := binary.BigEndian.Uint32(data[0:4])
	if count > 100000 {
		return nil, errors.New("too many certificates in list")
	}

	certs := make([]*types.Certificate, 0, count)
	offset := 4

	for i := uint32(0); i < count; i++ {
		if offset+4 > len(data) {
			return nil, errors.New("certificate list truncated")
		}

		certLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4

		if offset+int(certLen) > len(data) {
			return nil, errors.New("certificate data truncated")
		}

		cert, err := types.UnmarshalCertificate(data[offset : offset+int(certLen)])
		if err != nil {
			return nil, fmt.Errorf("unmarshal certificate %d: %w", i, err)
		}

		certs = append(certs, cert)
		offset += int(certLen)
	}

	return certs, nil
}

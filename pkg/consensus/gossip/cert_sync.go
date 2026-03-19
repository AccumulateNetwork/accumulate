// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// CertSyncRequest is a request to fetch missing certificates by digest.
type CertSyncRequest struct {
	// Digests are the certificate digests we need
	Digests []types.CertificateDigest
	// Requester is the public key of the requesting node
	Requester ed25519.PublicKey
	// RequestID is used for response correlation
	RequestID uint64
}

// CertSyncResponse is a response containing requested certificates.
type CertSyncResponse struct {
	// Certificates contains the requested certificates (if available)
	Certificates []*types.Certificate
	// RequestID is the correlation ID from the request
	RequestID uint64
	// Missing contains digests we don't have
	Missing []types.CertificateDigest
}

// Marshal serializes a CertSyncRequest.
// Format:
//   - RequestID (8 bytes)
//   - RequesterLen (4 bytes)
//   - Requester (variable, typically 32 bytes)
//   - NumDigests (4 bytes)
//   - Digests (32 bytes each)
func (r *CertSyncRequest) Marshal() ([]byte, error) {
	if len(r.Digests) > MaxSyncDigests {
		return nil, fmt.Errorf("too many digests: %d > %d", len(r.Digests), MaxSyncDigests)
	}

	size := 8 + 4 + len(r.Requester) + 4 + len(r.Digests)*32
	data := make([]byte, size)
	offset := 0

	// RequestID
	binary.BigEndian.PutUint64(data[offset:], r.RequestID)
	offset += 8

	// Requester
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Requester)))
	offset += 4
	copy(data[offset:], r.Requester)
	offset += len(r.Requester)

	// Digests
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Digests)))
	offset += 4

	for _, digest := range r.Digests {
		digest := digest // Create local copy to avoid rangevarref lint warning
		copy(data[offset:], digest[:])
		offset += 32
	}

	return data, nil
}

// UnmarshalCertSyncRequest deserializes a CertSyncRequest.
func UnmarshalCertSyncRequest(data []byte) (*CertSyncRequest, error) {
	if len(data) < 16 { // Minimum: 8 (requestID) + 4 (requesterLen) + 4 (numDigests)
		return nil, errors.New("cert sync request data too short")
	}

	offset := 0
	r := &CertSyncRequest{}

	// RequestID
	r.RequestID = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Requester
	requesterLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if requesterLen > 64 {
		return nil, errors.New("requester too large")
	}

	if offset+int(requesterLen) > len(data) {
		return nil, errors.New("cert sync request truncated: missing requester")
	}

	r.Requester = make([]byte, requesterLen)
	copy(r.Requester, data[offset:offset+int(requesterLen)])
	offset += int(requesterLen)

	// Digests
	if offset+4 > len(data) {
		return nil, errors.New("cert sync request truncated: missing digest count")
	}

	numDigests := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numDigests > MaxSyncDigests {
		return nil, fmt.Errorf("too many digests: %d > %d", numDigests, MaxSyncDigests)
	}

	r.Digests = make([]types.CertificateDigest, numDigests)
	for i := uint32(0); i < numDigests; i++ {
		if offset+32 > len(data) {
			return nil, errors.New("cert sync request truncated: missing digest")
		}
		copy(r.Digests[i][:], data[offset:offset+32])
		offset += 32
	}

	return r, nil
}

// Marshal serializes a CertSyncResponse.
// Format:
//   - RequestID (8 bytes)
//   - NumCertificates (4 bytes)
//   - For each certificate: [len (4 bytes)][data]
//   - NumMissing (4 bytes)
//   - Missing digests (32 bytes each)
func (r *CertSyncResponse) Marshal() ([]byte, error) {
	if len(r.Certificates) > MaxSyncCertificates {
		return nil, fmt.Errorf("too many certificates: %d > %d", len(r.Certificates), MaxSyncCertificates)
	}

	// Calculate size
	size := 8 + 4 // RequestID + NumCertificates

	certData := make([][]byte, len(r.Certificates))
	for i, cert := range r.Certificates {
		data, err := cert.Marshal()
		if err != nil {
			return nil, fmt.Errorf("marshal certificate %d: %w", i, err)
		}
		certData[i] = data
		size += 4 + len(data)
	}

	size += 4 + len(r.Missing)*32 // NumMissing + missing digests

	data := make([]byte, size)
	offset := 0

	// RequestID
	binary.BigEndian.PutUint64(data[offset:], r.RequestID)
	offset += 8

	// Certificates
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Certificates)))
	offset += 4

	for _, cd := range certData {
		binary.BigEndian.PutUint32(data[offset:], uint32(len(cd)))
		offset += 4
		copy(data[offset:], cd)
		offset += len(cd)
	}

	// Missing
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Missing)))
	offset += 4

	for _, digest := range r.Missing {
		digest := digest // Create local copy to avoid rangevarref lint warning
		copy(data[offset:], digest[:])
		offset += 32
	}

	return data, nil
}

// UnmarshalCertSyncResponse deserializes a CertSyncResponse.
func UnmarshalCertSyncResponse(data []byte) (*CertSyncResponse, error) {
	if len(data) < 16 { // Minimum: 8 (requestID) + 4 (numCerts) + 4 (numMissing)
		return nil, errors.New("cert sync response data too short")
	}

	offset := 0
	r := &CertSyncResponse{}

	// RequestID
	r.RequestID = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Certificates
	numCerts := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numCerts > MaxSyncCertificates {
		return nil, fmt.Errorf("too many certificates: %d > %d", numCerts, MaxSyncCertificates)
	}

	r.Certificates = make([]*types.Certificate, numCerts)
	for i := uint32(0); i < numCerts; i++ {
		if offset+4 > len(data) {
			return nil, errors.New("cert sync response truncated: missing cert length")
		}

		certLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4

		if certLen > MaxCertificateSize {
			return nil, errors.New("certificate too large")
		}

		if offset+int(certLen) > len(data) {
			return nil, errors.New("cert sync response truncated: missing cert data")
		}

		cert, err := types.UnmarshalCertificate(data[offset : offset+int(certLen)])
		if err != nil {
			return nil, fmt.Errorf("unmarshal certificate %d: %w", i, err)
		}
		r.Certificates[i] = cert
		offset += int(certLen)
	}

	// Missing
	if offset+4 > len(data) {
		return nil, errors.New("cert sync response truncated: missing count")
	}

	numMissing := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numMissing > MaxSyncDigests {
		return nil, fmt.Errorf("too many missing digests: %d > %d", numMissing, MaxSyncDigests)
	}

	r.Missing = make([]types.CertificateDigest, numMissing)
	for i := uint32(0); i < numMissing; i++ {
		if offset+32 > len(data) {
			return nil, errors.New("cert sync response truncated: missing digest")
		}
		copy(r.Missing[i][:], data[offset:offset+32])
		offset += 32
	}

	return r, nil
}

// Limits for sync messages.
const (
	// MaxSyncDigests is the maximum number of digests in a sync request.
	MaxSyncDigests = 1000
	// MaxSyncCertificates is the maximum number of certificates in a sync response.
	MaxSyncCertificates = 100
)

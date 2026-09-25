// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// DAGStore keeps, beside the checkpoint, what a restarted node needs to
// continue consensus from the round it stopped at (#4448): the certificates
// of the DAG's tail, the batches they name, and the header this node last
// authored. The DAG itself is memory-only; without these a partition that
// restarts as a whole holds no parent certificate anywhere and no node can
// author a header again.
//
// Certificates and batches are immutable and named by their digest, so each
// is written once, as its own file, and removed when no checkpoint kept
// names it: the write cost per block is what the block adds, not the window.
type DAGStore struct {
	mu    sync.Mutex
	dir   string
	known map[string]uint64 // file names present in dir, by write sequence
	seq   uint64
}

const (
	certPrefix   = "c-"
	batchPrefix  = "b-"
	authoredFile = "authored"
)

// OpenDAGStore opens (creating if needed) the store in dir.
func OpenDAGStore(dir string) (*DAGStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create DAG store: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read DAG store: %w", err)
	}
	s := &DAGStore{dir: dir, known: map[string]uint64{}}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".tmp") {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		if strings.HasPrefix(name, certPrefix) || strings.HasPrefix(name, batchPrefix) {
			s.known[name] = 0
		}
	}
	s.seq = 1
	return s, nil
}

// CertName and BatchName are the keys a checkpoint lists.
func CertName(d types.CertificateDigest) string { return certPrefix + hex.EncodeToString(d[:]) }
func BatchName(d types.BatchDigest) string      { return batchPrefix + hex.EncodeToString(d[:]) }

// writeFile writes data to name atomically. The caller holds mu.
func (s *DAGStore) writeFile(name string, data []byte) error {
	path := filepath.Join(s.dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// PutCertificate writes the certificate unless it is already stored.
func (s *DAGStore) PutCertificate(c *types.Certificate) (string, error) {
	name := CertName(c.Digest())
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.known[name]; ok {
		return name, nil
	}
	data, err := c.Marshal()
	if err != nil {
		return "", err
	}
	if err := s.writeFile(name, data); err != nil {
		return "", err
	}
	s.known[name] = s.seq
	s.seq++
	return name, nil
}

// PutBatch writes the batch unless it is already stored.
func (s *DAGStore) PutBatch(b *types.Batch) (string, error) {
	name := BatchName(b.Digest())
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.known[name]; ok {
		return name, nil
	}
	data, err := b.Marshal()
	if err != nil {
		return "", err
	}
	if err := s.writeFile(name, data); err != nil {
		return "", err
	}
	s.known[name] = s.seq
	s.seq++
	return name, nil
}

// HasBatch reports whether the batch is stored.
func (s *DAGStore) HasBatch(d types.BatchDigest) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.known[BatchName(d)]
	return ok
}

// Certificate reads a stored certificate by name.
func (s *DAGStore) Certificate(name string) (*types.Certificate, error) {
	if !strings.HasPrefix(name, certPrefix) {
		return nil, fmt.Errorf("not a certificate: %s", name)
	}
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return nil, err
	}
	return types.UnmarshalCertificate(data)
}

// Batch reads a stored batch by name.
func (s *DAGStore) Batch(name string) (*types.Batch, error) {
	if !strings.HasPrefix(name, batchPrefix) {
		return nil, fmt.Errorf("not a batch: %s", name)
	}
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return nil, err
	}
	return types.UnmarshalBatch(data)
}

// PutAuthored records the header this node authored, before it is
// broadcast. It is the one per-round write: a node that restarts must
// rebroadcast this header, never author a different one for its round
// (#4159 stall 3), and a block checkpoint can be several rounds older than
// the last header authored.
func (s *DAGStore) PutAuthored(h *types.Header) error {
	data, err := h.Marshal()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeFile(authoredFile, data)
}

// Authored returns the header this node last authored, or nil.
func (s *DAGStore) Authored() (*types.Header, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, authoredFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return types.UnmarshalHeader(data)
}

// Mark is a point in the store's write sequence: Retain spares everything
// written after it.
func (s *DAGStore) Mark() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

// Retain removes every stored certificate and batch written before mark
// that keep does not name. What was written after mark belongs to a round
// the caller did not see, and stays.
func (s *DAGStore) Retain(keep map[string]bool, mark uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, seq := range s.known {
		if keep[name] || seq >= mark {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, name)); err == nil || errors.Is(err, os.ErrNotExist) {
			delete(s.known, name)
		}
	}
}

// Names lists every stored certificate and batch.
func (s *DAGStore) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.known))
	for name := range s.known {
		out = append(out, name)
	}
	return out
}

// Len is the number of certificates and batches stored.
func (s *DAGStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.known)
}

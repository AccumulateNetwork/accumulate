// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// DAGStore keeps, beside the checkpoint, what a restarted node needs to
// continue consensus from the round it stopped at (#4448): the certificates
// of the DAG's tail, the batches they name, the header this node last
// authored, and the headers it voted for (#4590). The DAG itself is
// memory-only; without these a partition that restarts as a whole holds no
// parent certificate anywhere and no node can author a header again.
//
// Certificates and batches are immutable and named by their digest, so each
// is written once, as its own file, and removed when no checkpoint kept
// names it: the write cost per block is what the block adds, not the window.
type DAGStore struct {
	mu    sync.Mutex
	dir   string
	known map[string]uint64 // file names present in dir, by write sequence
	seq   uint64
	voted []VoteRecord
	// votedBy indexes voted by author and round: PutVoted otherwise scanned
	// every record on every vote.
	votedBy map[authorRoundKey]types.HeaderDigest
	// votedFloor is the round below which the records have been dropped,
	// written at the head of the file and read back with the records: a
	// restarted node otherwise knew the records it kept but not that lower
	// rounds had been voted in and dropped (#4590). It only rises.
	votedFloor types.Round

	// authoredMu orders the authored header's writes, apart from mu: its
	// two syncs must not hold up the certificate written at each insert
	// (#4595).
	authoredMu sync.Mutex
}

// authorRoundKey keys votedBy.
type authorRoundKey struct {
	author string
	round  types.Round
}

const (
	certPrefix   = "c-"
	batchPrefix  = "b-"
	authoredFile = "authored"
	votedFile    = "voted"
)

// VoteRecord is a header this node voted for: the only one it votes for
// from that author at that round (#4590).
type VoteRecord struct {
	Author ed25519.PublicKey
	Round  types.Round
	Header types.HeaderDigest
}

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
	if err := s.readVoted(); err != nil {
		return nil, fmt.Errorf("read vote records: %w", err)
	}
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

// fsync flushes a file, or a directory, to the device. A variable so the
// test of the authored header's write can see each sync and its order.
var fsync = func(f *os.File) error { return f.Sync() }

// writeFileSynced is writeFile with the file and then the directory synced
// before it returns, for a write whose loss is not merely a loss (#4592):
// the authored header is broadcast once it returns, and a power loss that
// brought the previous file back would have the node author the round
// again. The caller holds authoredMu.
func (s *DAGStore) writeFileSynced(name string, data []byte) error {
	path := filepath.Join(s.dir, name)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := fsync(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	d, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return fsync(d)
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
	s.authoredMu.Lock()
	defer s.authoredMu.Unlock()
	// Synced, alone in this store (DIFFERENCES, C9): a certificate or batch
	// lost to a power loss costs a fetch, this file an equivocation.
	return s.writeFileSynced(authoredFile, data)
}

// Authored returns the header this node last authored, or nil if it has
// authored none. Any other failure names the file: a node that cannot read
// it does not know which round never to author again, and must not start
// (#4592).
func (s *DAGStore) Authored() (*types.Header, error) {
	path := filepath.Join(s.dir, authoredFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h, err := types.UnmarshalHeader(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return h, nil
}

// PutVoted records, before the vote is signed, that this node votes for the
// header from author at round (#4590): one line appended to the voted file.
// Not synced, unlike the authored header (DIFFERENCES, C9).
func (s *DAGStore) PutVoted(author ed25519.PublicKey, round types.Round, digest types.HeaderDigest) error {
	r := VoteRecord{Author: author, Round: round, Header: digest}
	s.mu.Lock()
	defer s.mu.Unlock()
	if have, ok := s.votedBy[r.key()]; ok && have == r.Header {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(s.dir, votedFile), os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	data, err := startLine(f, r.line())
	if err == nil {
		_, err = f.Write(data)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	s.addVoted(r)
	return nil
}

// startLine returns line, preceded by a newline when f does not end in one:
// a write that failed part way left a partial line, and a record appended
// to it would be read as one line of four fields and dropped, the vote it
// records sent (#4590 review).
func startLine(f *os.File, line []byte) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return line, nil
	}
	var last [1]byte
	if _, err := f.ReadAt(last[:], info.Size()-1); err != nil {
		return nil, err
	}
	if last[0] == '\n' {
		return line, nil
	}
	return append([]byte{'\n'}, line...), nil
}

// addVoted appends r to the records and indexes it.
func (s *DAGStore) addVoted(r VoteRecord) {
	if s.votedBy == nil {
		s.votedBy = make(map[authorRoundKey]types.HeaderDigest)
	}
	s.voted = append(s.voted, r)
	s.votedBy[r.key()] = r.Header
}

func (r VoteRecord) key() authorRoundKey {
	return authorRoundKey{string(r.Author), r.Round}
}

// Voted returns the headers this node voted for, oldest first.
func (s *DAGStore) Voted() []VoteRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]VoteRecord(nil), s.voted...)
}

// VotedFloor returns the round below which the vote records have been
// dropped: no header below it may be voted, as the record of what was voted
// there is gone (#4590).
func (s *DAGStore) VotedFloor() types.Round {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.votedFloor
}

// RetainVoted drops the vote records of rounds below floor and writes floor
// at the head of the file, so a restarted node knows the rounds it can no
// longer tell its votes in. The floor only rises: a lower floor, as the
// first checkpoint saved after a restart asks for, changes nothing. If the
// file cannot be rewritten the records and the old floor are all kept, in
// memory and on disk, and the next checkpoint tries again.
func (s *DAGStore) RetainVoted(floor types.Round) {
	s.mu.Lock()
	defer s.mu.Unlock()
	floor = max(floor, s.votedFloor)
	var kept []VoteRecord
	for _, r := range s.voted {
		if r.Round >= floor {
			kept = append(kept, r)
		}
	}
	if floor == s.votedFloor && len(kept) == len(s.voted) {
		return
	}
	if err := s.writeFile(votedFile, votedData(floor, kept)); err != nil {
		slog.Error("Cannot rewrite the vote records; keeping them all",
			"dir", s.dir, "floor", floor, "error", err)
		return
	}
	s.votedFloor = floor
	s.voted, s.votedBy = nil, nil
	for _, r := range kept {
		s.addVoted(r)
	}
}

// votedData is the voted file holding floor and records: the floor line
// first, when there is a floor, then one record per line.
func votedData(floor types.Round, records []VoteRecord) []byte {
	var data []byte
	if floor > 0 {
		data = fmt.Appendf(data, "%s %d\n", floorWord, floor)
	}
	for _, r := range records {
		data = append(data, r.line()...)
	}
	return data
}

// line is the record as the voted file holds it: round, author and header
// digest in hex, one record per line.
func (r VoteRecord) line() []byte {
	return []byte(fmt.Sprintf("%d %x %x\n", r.Round, []byte(r.Author), r.Header[:]))
}

// floorWord starts the line of the voted file that holds the prune floor.
// An older binary reads a file with one as if it had no floor: the line
// has two fields, not a record's three.
const floorWord = "floor"

// readVoted loads the voted file: the prune floor, if a checkpoint has
// written one, and the records. A line that is not either is dropped and
// logged; a last line cut short by a crash is dropped and the file
// rewritten without it, so the next record starts a line.
func (s *DAGStore) readVoted() error {
	data, err := os.ReadFile(filepath.Join(s.dir, votedFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for lineNo := 1; sc.Scan(); lineNo++ {
		if floor, ok := parseVotedFloor(sc.Text()); ok {
			s.votedFloor = max(s.votedFloor, floor)
			continue
		}
		r, ok := parseVoteRecord(sc.Text())
		if !ok {
			slog.Warn("Dropping a line of the vote records that is not a record",
				"dir", s.dir, "line", lineNo)
			continue
		}
		s.addVoted(r)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return s.writeFile(votedFile, votedData(s.votedFloor, s.voted))
	}
	return nil
}

// parseVotedFloor reads the floor line of the voted file, as votedData
// writes it.
func parseVotedFloor(text string) (types.Round, bool) {
	f := strings.Fields(text)
	if len(f) != 2 || f[0] != floorWord {
		return 0, false
	}
	floor, err := strconv.ParseUint(f[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return types.Round(floor), true
}

// parseVoteRecord reads one line of the voted file, as line writes it.
func parseVoteRecord(text string) (VoteRecord, bool) {
	f := strings.Fields(text)
	if len(f) != 3 {
		return VoteRecord{}, false
	}
	round, err := strconv.ParseUint(f[0], 10, 64)
	if err != nil {
		return VoteRecord{}, false
	}
	author, err := hex.DecodeString(f[1])
	if err != nil || len(author) != ed25519.PublicKeySize {
		return VoteRecord{}, false
	}
	digest, err := hex.DecodeString(f[2])
	if err != nil || len(digest) != len(types.HeaderDigest{}) {
		return VoteRecord{}, false
	}
	r := VoteRecord{Author: author, Round: types.Round(round)}
	copy(r.Header[:], digest)
	return r, true
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

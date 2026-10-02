// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// sectionAdapter executes blocks over a state whose root is a hash chain of
// the groups it executed, by number: block n's root is H(root n-1, n, the
// group's leader round). A node that executes the validators' groups as the
// validators' numbers has the validators' roots; one that executes a group
// under another number does not. A round in fail is a group whose block
// reads a section of the state tree this node has not loaded: until the
// count runs out, its block is refused at commit as the store refuses it --
// NotReady, nothing committed.
type sectionAdapter struct {
	commitAdapter
	mu2      sync.Mutex
	fail     map[types.Round]int
	attempts []string // "index/round" of every block the service asked for
	root     [32]byte
}

func (a *sectionAdapter) ProduceBlock(ctx context.Context, p adapter.BlockParams) ([32]byte, error) {
	a.mu2.Lock()
	defer a.mu2.Unlock()
	a.attempts = append(a.attempts, fmt.Sprintf("%d/%d", p.Index, p.LeaderRound))
	if a.fail[p.LeaderRound] != 0 {
		if a.fail[p.LeaderRound] > 0 {
			a.fail[p.LeaderRound]--
		}
		return [32]byte{}, fmt.Errorf("commit block: %w",
			errors.NotReady.WithFormat("section 1017 of the state tree, which acc://dave/book/1 is in, could not be loaded: no peer answered"))
	}
	a.root = chainRoot(a.root, p.Index, p.LeaderRound)
	a.hash = a.root
	return a.commitAdapter.ProduceBlock(ctx, p)
}

func (a *sectionAdapter) StateHash() [32]byte {
	a.mu2.Lock()
	defer a.mu2.Unlock()
	return a.root
}

func chainRoot(prev [32]byte, index uint64, round types.Round) [32]byte {
	return sha256.Sum256(fmt.Appendf(prev[:], "%d/%d", index, round))
}

func newSectionService(t *testing.T, fail map[types.Round]int) (*Service, *sectionAdapter, ed25519.PublicKey) {
	svc, ca, author := newCommitService(t, 1)
	sa := &sectionAdapter{commitAdapter: commitAdapter{hash: ca.hash}, fail: fail}
	svc.adapter = sa
	return svc, sa, author
}

// TestAHeldBlockIsHeldNotSkipped — #4485 review, finding 3; executor spec,
// "Sync", §3a: a block whose section could not be loaded is held. After the
// handoff, the block of round 2 is refused twice -- no peer answers its
// section -- and then its peer serves. The service must not go on to the
// next group under the held block's number: it holds round 2 as block 1,
// produces it when the load succeeds, and produces round 4 as block 2. The
// node's root is then the root of the validators, who executed round 2 as
// block 1 and round 4 as block 2.
func TestAHeldBlockIsHeldNotSkipped(t *testing.T) {
	setHold(t, time.Second, time.Millisecond)
	svc, sa, author := newSectionService(t, map[types.Round]int{2: 2})

	_, held := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0), nil)))
	_, err := svc.processCommittedGroup(group(commitCert(author, 4, time.Unix(101, 0), nil)))
	require.NoError(t, err)

	require.NotContains(t, sa.attempts, "1/4", "the next group was executed under the held block's number")
	require.NoError(t, held, "a block held until its section loaded was not produced")
	require.Equal(t, []string{"1/2", "1/2", "1/2", "2/4"}, sa.attempts,
		"the held block was not retried until its section loaded, or the next group did not follow it")
	require.Equal(t, uint64(2), svc.lastBlockIndex)
	want := chainRoot(chainRoot([32]byte{}, 1, 2), 2, 4)
	require.Equal(t, want, sa.StateHash(), "the node's root after the held block is not the validators'")
}

// TestAHeldBlockPastItsBoundCollectsAgain — #4485 review, finding 3. A block
// whose section no peer serves within the hold's bound is not produced, and
// neither is anything after it: the service collects again, its buffer
// beginning with the held group, and says which block it held, so the join
// demotes the node and syncs again (join.HeldBuffer).
func TestAHeldBlockPastItsBoundCollectsAgain(t *testing.T) {
	setHold(t, 50*time.Millisecond, time.Millisecond)
	svc, sa, author := newSectionService(t, map[types.Round]int{2: -1})
	svc.adapter = &collectingSectionAdapter{sa}

	_, err := svc.processCommittedGroup(group(commitCert(author, 2, time.Unix(100, 0), nil)))
	require.Error(t, err, "a block held past its bound reported success")
	require.True(t, svc.Collecting(), "the service did not collect again after holding a block past its bound")
	require.Equal(t, uint64(1), svc.HeldBlock(), "the service does not say which block it held")
	require.Equal(t, uint64(0), svc.lastBlockIndex, "a held block moved the block index")

	_, err = svc.processCommittedGroup(group(commitCert(author, 4, time.Unix(101, 0), nil)))
	require.NoError(t, err)
	for _, a := range sa.attempts {
		require.Equal(t, "1/2", a, "a group after the held one was executed")
	}
	buf := svc.Buffered()
	require.Len(t, buf, 2, "the buffer does not hold the held group and the one after it")
	require.Equal(t, types.Round(2), buf[0].Round(), "the held group is not first in the buffer")
	require.Equal(t, types.Round(4), buf[1].Round())
}

// collectingSectionAdapter is a sectionAdapter that can collect a group
// without executing it, as the executor bridge can.
type collectingSectionAdapter struct{ *sectionAdapter }

func (*collectingSectionAdapter) CollectBlock(context.Context, adapter.BlockParams) (*execute.CollectedBlock, error) {
	return new(execute.CollectedBlock), nil
}

func setHold(t *testing.T, bound, retry time.Duration) {
	b, r := heldBlockBound, heldBlockRetry
	heldBlockBound, heldBlockRetry = bound, retry
	t.Cleanup(func() { heldBlockBound, heldBlockRetry = b, r })
}

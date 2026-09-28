// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

// The two checkpoint files a validator keeps: the position of the block
// being produced, and of the block before it.
const (
	CheckpointFile     = "consensus-checkpoint.json"
	PrevCheckpointFile = "consensus-checkpoint.prev.json"
)

// LoadResume loads the checkpoints kept in dir for partition and chooses the
// one a restarted node resumes from (ResumeFrom). It returns nil when none
// will do.
func LoadResume(dir, partition string, lastBlock uint64) *Checkpoint {
	load := func(file string) *Checkpoint {
		store := NewStore(dir)
		store.SetFilename(file)
		cp, err := store.Load()
		if err != nil || cp.Partition != partition {
			return nil
		}
		return cp
	}
	return ResumeFrom(lastBlock, load(CheckpointFile), load(PrevCheckpointFile))
}

// ResumeFrom chooses, of the checkpoint for the block being produced (cur)
// and the one for the block before it (prev), the consensus position a
// restarted node resumes from, given the executor's last block. The node
// resumes after the chosen checkpoint's BlockIndex.
//
// A checkpoint is written before its block is produced, so the one whose
// block is the executor's last block is the position that produced it.
//
// The executor keeps no empty block: its last block is the last block that
// carried anything, and the blocks produced after it may be empty and
// discarded. A BVN whose last blocks carried nothing therefore restarts with
// both checkpoints above its executor's last block (#4448). Then prev is
// still a position to resume from: cur was written only once prev's block
// had been produced, so every block through prev is produced, and the ones
// above the executor's last block carried nothing. cur is not: its block may
// never have been produced, and resuming there would skip its group.
func ResumeFrom(lastBlock uint64, cur, prev *Checkpoint) *Checkpoint {
	for _, cp := range []*Checkpoint{cur, prev} {
		if cp != nil && cp.BlockIndex == lastBlock {
			return cp
		}
	}
	if cur != nil && prev != nil && prev.BlockIndex+1 == cur.BlockIndex && prev.BlockIndex > lastBlock {
		return prev
	}
	return nil
}

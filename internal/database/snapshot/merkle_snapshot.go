package snapshot

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// AddEntry adds an entry to the snapshot as if it were added to the chain.
// AddEntry's logic must mirror MerkleManager.AddHash.
func (c *Chain) AddEntry(hash []byte) {
	var markFreq = int64(1) << int64(c.MarkPower)
	var markMask = markFreq - 1
	switch (c.Head.Count + 1) & markMask {
	case 0:
		c.Head.AddToMerkleTree(hash)
		c.MarkPoints = append(c.MarkPoints, c.Head.Copy()) // Save the mark point
	case 1:
		c.Head.HashList = c.Head.HashList[:0]
		fallthrough
	default:
		c.Head.AddToMerkleTree(hash)
	}
}

func CollectChain(c *database.MerkleManager) (*Chain, error) {
	head, err := c.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load head: %w", err)
	}

	s := new(Chain)
	s.Name = c.Name()
	s.Type = c.Type()
	s.MarkPower = uint64(c.MarkPower())
	s.Head = head

	// Collect the mark points
	lastMark := head.Count &^ c.MarkMask()
	for i := c.MarkFreq(); i <= lastMark; i += c.MarkFreq() {
		state, err := c.States(uint64(i - 1)).Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			continue
		default:
			return nil, errors.UnknownError.WithFormat("load mark point %d: %w", i, err)
		}

		s.MarkPoints = append(s.MarkPoints, state)
		if i != state.Count {
			return nil, errors.InternalError.WithFormat("expected mark point %d but count is %d", i-1, state.Count)
		}
	}
	return s, nil
}

func (c *Chain) RestoreSnapshot(s *database.MerkleManager) error {
	err := c.RestoreHead(s)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = c.RestoreMarkPointRange(s, 0, len(c.MarkPoints))
	return errors.UnknownError.Wrap(err)
}

func (s *Chain) RestoreHead(c *database.MerkleManager) error {
	// Ensure the chain is empty
	head, err := c.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load head: %w", err)
	}
	if head.Count > 0 {
		return errors.Conflict.WithFormat("cannot restore onto existing chain")
	}

	if s.MarkPower != uint64(c.MarkPower()) {
		// It is possible to handle this but I'm not going to bother writing the
		// code unless we need it
		return errors.Conflict.WithFormat("mark power conflict: %d != %d", s.MarkPower, c.MarkPower())
	}

	err = c.Head().Put(s.Head)
	if err != nil {
		return errors.UnknownError.WithFormat("store head: %w", err)
	}

	return nil
}

func (s *Chain) RestoreMarkPointRange(c *database.MerkleManager, start, end int) error {
	for _, state := range s.MarkPoints[start:end] {
		if state == new(database.MerkleState) {
			continue
		}

		if state.Count&c.MarkMask() != 0 {
			return errors.Conflict.WithFormat("mark power conflict: count %d does not match mark power %d", state.Count, c.MarkPower())
		}

		err := c.States(uint64(state.Count - 1)).Put(state)
		if err != nil {
			return errors.UnknownError.WithFormat("store mark point %d: %w", state.Count, err)
		}
	}
	return nil
}

func (s *Chain) RestoreElementIndexFromHead(c *database.MerkleManager) error {
	lastMark := s.Head.Count &^ c.MarkMask()
	for i, h := range s.Head.HashList {
		err := c.ElementIndex(h).Put(uint64(lastMark) + uint64(i))
		if err != nil {
			return errors.UnknownError.WithFormat("store element index: %w", err)
		}
	}
	return nil
}

func (s *Chain) RestoreElementIndexFromMarkPoints(c *database.MerkleManager, start, end int) error {
	for _, state := range s.MarkPoints[start:end] {
		if state == new(database.MerkleState) {
			continue
		}

		if state.Count&c.MarkMask() != 0 {
			return errors.Conflict.WithFormat("mark power conflict: count %d does not match mark power %d", state.Count, c.MarkPower())
		}

		for i, h := range state.HashList {
			err := c.ElementIndex(h).Put(uint64(state.Count) + uint64(i))
			if err != nil {
				return errors.UnknownError.WithFormat("store element index: %w", err)
			}
		}
	}
	return nil
}

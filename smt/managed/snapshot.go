package managed

import "gitlab.com/accumulatenetwork/accumulate/internal/errors"

// AddEntry adds an entry to the snapshot as if it were added to the chain.
// AddEntry's logic must mirror MerkleManager.AddHash.
func (c *Snapshot) AddEntry(hash Hash) {
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

func (c *Chain) CollectSnapshot() (*Snapshot, error) {
	head, err := c.Head().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load head: %w", err)
	}

	s := new(Snapshot)
	s.Name = c.name
	s.Type = c.typ
	s.MarkPower = uint64(c.markPower)
	s.Head = head

	// Collect the mark points
	lastMark := head.Count &^ c.markMask
	for i := c.markFreq; i <= lastMark; i += c.markFreq {
		state, err := c.States(uint64(i - 1)).Get()
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.StatusNotFound):
			continue
		default:
			return nil, errors.Format(errors.StatusUnknownError, "load mark point %d: %w", i, err)
		}

		s.MarkPoints = append(s.MarkPoints, state)
		if i != state.Count {
			return nil, errors.Format(errors.StatusInternalError, "expected mark point %d but count is %d", i-1, state.Count)
		}
	}
	return s, nil
}

func (c *Chain) RestoreSnapshot(s *Snapshot) error {
	err := c.RestoreHead(s)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = c.RestoreMarkPointRange(s, 0, len(s.MarkPoints))
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (c *Chain) RestoreHead(s *Snapshot) error {
	// Ensure the chain is empty
	head, err := c.Head().Get()
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "load head: %w", err)
	}
	if head.Count > 0 {
		return errors.Format(errors.StatusConflict, "cannot restore onto existing chain")
	}

	if s.MarkPower != uint64(c.markPower) {
		// It is possible to handle this but I'm not going to bother writing the
		// code unless we need it
		return errors.Format(errors.StatusConflict, "mark power conflict: %d != %d", s.MarkPower, c.markPower)
	}

	err = c.Head().Put(s.Head)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "store head: %w", err)
	}

	return nil
}

func (c *Chain) RestoreMarkPointRange(s *Snapshot, start, end int) error {
	for _, state := range s.MarkPoints[start:end] {
		if state == new(MerkleState) {
			continue
		}

		if state.Count&c.markMask != 0 {
			return errors.Format(errors.StatusConflict, "mark power conflict: count %d does not match mark power %d", state.Count, c.markPower)
		}

		err := c.States(uint64(state.Count - 1)).Put(state)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "store mark point %d: %w", state.Count, err)
		}
	}
	return nil
}

func (c *Chain) RestoreElementIndexFromHead(s *Snapshot) error {
	lastMark := s.Head.Count &^ c.markMask
	for i, h := range s.Head.HashList {
		err := c.ElementIndex(h).Put(uint64(lastMark) + uint64(i))
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "store element index: %w", err)
		}
	}
	return nil
}

func (c *Chain) RestoreElementIndexFromMarkPoints(s *Snapshot, start, end int) error {
	for _, state := range s.MarkPoints[start:end] {
		if state == new(MerkleState) {
			continue
		}

		if state.Count&c.markMask != 0 {
			return errors.Format(errors.StatusConflict, "mark power conflict: count %d does not match mark power %d", state.Count, c.markPower)
		}

		for i, h := range state.HashList {
			err := c.ElementIndex(h).Put(uint64(state.Count) + uint64(i))
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "store element index: %w", err)
			}
		}
	}
	return nil
}

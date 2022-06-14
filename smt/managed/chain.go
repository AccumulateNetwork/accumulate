package managed

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

func (c *Chain) AddEntry(entry []byte, unique bool) error {
	return c.AddHash(entry, unique)
}

func (c *Chain) Entries(start int64, end int64) ([][]byte, error) {
	state, err := c.Head().Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	if end > state.Count {
		end = state.Count
	}

	if end < start {
		return nil, errors.New(errors.StatusBadRequest, "invalid range: start is greater than end")
	}

	// GetRange will not cross mark point boundaries, so we may need to call it
	// multiple times
	entries := make([][]byte, 0, end-start)
	for start < end {
		h, err := c.GetRange(start, end)
		if err != nil {
			return nil, err
		}

		for i := range h {
			entries = append(entries, h[i])
		}
		start += int64(len(h))
	}

	return entries, nil
}

// Receipt builds a receipt from one index to another
func (c *Chain) Receipt(from, to int64) (*Receipt, error) {
	state, err := c.Head().Get()
	if err != nil {
		return nil, err
	}
	if from < 0 {
		return nil, fmt.Errorf("invalid range: from (%d) < 0", from)
	}
	if to < 0 {
		return nil, fmt.Errorf("invalid range: to (%d) < 0", to)
	}
	if from > state.Count {
		return nil, fmt.Errorf("invalid range: from (%d) > height (%d)", from, state.Count)
	}
	if to > state.Count {
		return nil, fmt.Errorf("invalid range: to (%d) > height (%d)", to, state.Count)
	}
	if from > to {
		return nil, fmt.Errorf("invalid range: from (%d) > to (%d)", from, to)
	}

	r := NewReceipt(c)
	r.StartIndex = from
	r.EndIndex = to
	r.Start, err = c.Element(uint64(from)).Get()
	if err != nil {
		return nil, err
	}
	r.End, err = c.Element(uint64(to)).Get()
	if err != nil {
		return nil, err
	}

	// If this is the first element in the Merkle Tree, we are already done
	if from == 0 && to == 0 {
		r.Anchor = r.Start
		return r, nil
	}

	err = r.BuildReceipt()
	if err != nil {
		return nil, err
	}

	return r, nil
}

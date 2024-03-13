// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --out recorder_enums_gen.go --package simulator recorder_enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --out recorder_types_gen.go --package simulator recorder_types.yml

type recordSectionType int

func newRecorder(w io.WriteSeeker) *recorder {
	sw := ioutil.NewSegmentedWriter[recordSectionType](w)
	return &recorder{*sw}
}

type recorder struct {
	ioutil.SegmentedWriter[recordSectionType, *recordSectionType]
}

func (r *recorder) WriteHeader(h *recordHeader) error {
	b, err := h.MarshalBinary()
	if err != nil {
		return errors.EncodingError.WithFormat("encode: %w", err)
	}
	w, err := r.Open(recordSectionTypeHeader)
	if err != nil {
		return errors.UnknownError.WithFormat("open: %w", err)
	}
	_, err = w.Write(b)
	if err != nil {
		return errors.UnknownError.WithFormat("write: %w", err)
	}
	err = w.Close()
	if err != nil {
		return errors.UnknownError.WithFormat("close: %w", err)
	}
	return nil
}

func (r *recorder) DidInit(snapshot ioutil.SectionReader) error {
	_, err := snapshot.Seek(0, io.SeekStart)
	if err != nil {
		return errors.UnknownError.WithFormat("seek start: %w", err)
	}
	w, err := r.Open(recordSectionTypeSnapshot)
	if err != nil {
		return errors.UnknownError.WithFormat("open: %w", err)
	}
	_, err = io.Copy(w, snapshot)
	if err != nil {
		return errors.UnknownError.WithFormat("copy: %w", err)
	}
	err = w.Close()
	if err != nil {
		return errors.UnknownError.WithFormat("close: %w", err)
	}
	return nil
}

func (r *recorder) DidExecuteBlock(state execute.BlockState, submissions []*messaging.Envelope) error {
	ci, err := json.Marshal(state.Params().CommitInfo)
	if err != nil {
		return errors.EncodingError.WithFormat("encode commit info: %w", err)
	}
	ev, err := json.Marshal(state.Params().Evidence)
	if err != nil {
		return errors.EncodingError.WithFormat("encode evidence: %w", err)
	}
	b := &recordBlock{
		IsLeader:    state.Params().IsLeader,
		Index:       state.Params().Index,
		Time:        state.Params().Time,
		CommitInfo:  ci,
		Evidence:    ev,
		Submissions: submissions,
	}

	if !state.IsEmpty() {
		err = state.ChangeSet().Walk(database.WalkOptions{
			Values:        true,
			Modified:      true,
			IgnoreIndices: true,
		}, func(r database.Record) (skip bool, err error) {
			rv, ok := r.(database.Value)
			if !ok {
				return false, nil
			}
			v, _, err := rv.GetValue()
			if err != nil {
				return false, errors.UnknownError.Wrap(err)
			}
			bv, err := v.MarshalBinary()
			if err != nil {
				return false, errors.UnknownError.Wrap(err)
			}
			b.Changes = append(b.Changes, &recordChange{
				Key:   r.Key(),
				Value: bv,
			})
			return false, nil
		})
		if err != nil {
			return errors.UnknownError.WithFormat("walk changes: %w", err)
		}
	}

	v, err := b.MarshalBinary()
	if err != nil {
		return errors.EncodingError.WithFormat("encode: %w", err)
	}
	w, err := r.Open(recordSectionTypeBlock)
	if err != nil {
		return errors.UnknownError.WithFormat("open: %w", err)
	}
	_, err = w.Write(v)
	if err != nil {
		return errors.UnknownError.WithFormat("write: %w", err)
	}
	err = w.Close()
	if err != nil {
		return errors.UnknownError.WithFormat("close: %w", err)
	}
	return nil
}

func DumpRecording(rd ioutil.SectionReader) error {
	r := ioutil.NewSegmentedReader[recordSectionType](rd)
	for {
		s, err := r.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		r, err := s.Open()
		if err != nil {
			return err
		}

		fmt.Printf("%s section\n", s.Type())
		switch s.Type() {
		case recordSectionTypeHeader:
			h := new(recordHeader)
			err = h.UnmarshalBinaryFrom(r)
			if err != nil {
				return err
			}
			fmt.Printf("  Partition %s (%v) node %v\n", h.Partition.ID, h.Partition.Type, h.NodeID)

		case recordSectionTypeBlock:
			b := new(recordBlock)
			err = b.UnmarshalBinaryFrom(r)
			if err != nil {
				return err
			}
			fmt.Printf("  Block %d @ %v\n", b.Index, b.Time)
			for _, s := range b.Submissions {
				b, err := json.Marshal(s)
				if err != nil {
					return err
				}
				fmt.Printf("  Submission %s\n", b)
			}
			sort.Slice(b.Changes, func(i, j int) bool {
				a, b := b.Changes[i], b.Changes[j]
				return a.Key.Compare(b.Key) < 0
			})
			for _, c := range b.Changes {
				fmt.Printf("  Changed %v\n", c.Key)
			}
		}
	}
}

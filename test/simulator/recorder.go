// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
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
	return r.writeValue(recordSectionTypeHeader, h)
}

func (r *recorder) DidInit(snapshot ioutil.SectionReader) error {
	_, err := snapshot.Seek(0, io.SeekStart)
	if err != nil {
		return errors.UnknownError.WithFormat("seek start: %w", err)
	}
	return r.writeSection(recordSectionTypeSnapshot, snapshot)
}

func (r *recorder) DidReceiveMessages(messages []consensus.Message) error {
	return r.writeValue(recordSectionTypeMessages, &recordMessages{Messages: messages})
}

func (r *recorder) DidCommitBlock(state execute.BlockState) error {
	ci, err := json.Marshal(state.Params().CommitInfo)
	if err != nil {
		return errors.EncodingError.WithFormat("encode commit info: %w", err)
	}
	ev, err := json.Marshal(state.Params().Evidence)
	if err != nil {
		return errors.EncodingError.WithFormat("encode evidence: %w", err)
	}
	b := &recordBlock{
		IsLeader:   state.Params().IsLeader,
		Index:      state.Params().Index,
		Time:       state.Params().Time,
		CommitInfo: ci,
		Evidence:   ev,
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

	return r.writeValue(recordSectionTypeBlock, b)
}

func (r *recorder) writeValue(typ recordSectionType, val encoding.BinaryValue) error {
	b, err := val.MarshalBinary()
	if err != nil {
		return errors.EncodingError.WithFormat("encode: %w", err)
	}
	return r.writeSection(typ, bytes.NewReader(b))
}

func (r *recorder) writeSection(typ recordSectionType, data io.Reader) error {
	w, err := r.Open(typ)
	if err != nil {
		return errors.UnknownError.WithFormat("open: %w", err)
	}
	_, err = io.Copy(w, data)
	if err != nil {
		return errors.UnknownError.WithFormat("copy: %w", err)
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

		case recordSectionTypeMessages:
			m := new(recordMessages)
			err = m.UnmarshalBinaryFrom(r)
			if err != nil {
				return err
			}
			for _, m := range m.Messages {
				switch m := m.(type) {
				case consensus.NodeMessage:
					s := m.SenderID()
					fmt.Printf("  Message %v for %s from %x\n", m.Type(), m.PartitionID(), s[:4])
				case consensus.NetworkMessage:
					fmt.Printf("  Message %v for %s\n", m.Type(), m.PartitionID())
				default:
					fmt.Printf("  Message %v\n", m.Type())
				}
			}

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

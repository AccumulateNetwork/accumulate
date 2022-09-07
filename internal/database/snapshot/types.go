package snapshot

//go:generate go run ../../../tools/cmd/gen-enum --package snapshot enums.yml
//go:generate go run ../../../tools/cmd/gen-types --package snapshot types.yml

type SectionType uint64

type ChainState = Chain

/*
type TmStateSnapshot struct {
	Height          uint64
	Blocks          []*tmproto.LightBlock
	ConsensusParams *tmproto.ConsensusParams
}

func (s *TmStateSnapshot) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)
	writer.WriteUint(1, s.Height)
	if len(s.Blocks) > 0 {
		for _, blk := range s.Blocks {
			writer.WriteValue(2, blk.Marshal)
		}
	}
	writer.WriteValue(3, s.ConsensusParams.Marshal)
	return buffer.Bytes(), nil
}

func (s *TmStateSnapshot) UnmarshalBinary(data []byte) error {
	reader := encoding.NewReader(bytes.NewReader(data))
	if sh, ok := reader.ReadUint(1); ok {
		s.Height = sh
	} else {
		return errors.New(errors.StatusUnknownError, "UnmarshalBinary error")
	}

	for {
		blkProto := &tmproto.LightBlock{}
		if ok := reader.ReadValue(2, blkProto.Unmarshal); !ok {
			break
		}
		s.Blocks = append(s.Blocks, blkProto)
	}

	cpProto := new(tmproto.ConsensusParams)
	if ok := reader.ReadValue(3, cpProto.Unmarshal); !ok {
		return errors.New(errors.StatusUnknownError, "UnmarshalBinary error")
	}

	s.ConsensusParams = cpProto
	return nil
}
*/

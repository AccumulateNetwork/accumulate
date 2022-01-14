package chain

func (b *blockMetadata) Empty() bool {
	return b.Delivered == 0 &&
		b.SynthSigned == 0 &&
		b.SynthSent == 0
}

type blockMetadata struct {
	Delivered   uint64 `json:"delivered,omitempty" form:"delivered" query:"delivered" validate:"required"`
	SynthSigned uint64 `json:"synthSigned,omitempty" form:"synthSigned" query:"synthSigned" validate:"required"`
	SynthSent   uint64 `json:"synthSent,omitempty" form:"synthSent" query:"synthSent" validate:"required"`
}

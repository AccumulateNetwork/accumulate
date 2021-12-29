package chain

//go:generate go run ../../tools/cmd/gentypes --package chain types.yml

func (b *BlockMetadata) Empty() bool {
	return b.Deliver.Empty() &&
		b.Delivered == 0 &&
		b.SynthSigned == 0 &&
		b.SynthSent == 0
}

func (d *DeliverMetadata) Empty() bool {
	return len(d.Updated) == 0 && len(d.Submitted) == 0
}

func (d *DeliverMetadata) Append(e DeliverMetadata) {
	d.Updated = append(d.Updated, e.Updated...)
	d.Submitted = append(d.Submitted, e.Submitted...)
}

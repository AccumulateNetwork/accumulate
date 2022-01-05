package protocol

import "time"

// EBlock represents a Factom Entry Block.
type EBlock struct {
	// DBlock.Get populates the ChainID, KeyMR, Height and Timestamp.
	ChainID   [32]byte
	KeyMR     [32]byte  // Computed
	Timestamp time.Time // Established by DBlock
	Height    uint32

	FullHash [32]byte // Computed

	// Unmarshaled
	PrevKeyMR    [32]byte
	PrevFullHash [32]byte
	BodyMR       [32]byte
	Sequence     uint32
	ObjectCount  uint32

	// EBlock.Get populates the Entries with their Hash and Timestamp.
	Entries []LiteDataEntry
}

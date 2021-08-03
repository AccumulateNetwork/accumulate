package security


SignatorEd25519 {
	Public [32]Byte
}

var _ Signator = (nil).(*SignatorEd25519)


// Signator
// Signs or authorizes token transactions
type Signator interface {
	SignatorHash() [32]byte
	Validate(signature Signature, data []byte) bool
	Type() int
}


type Group struct {
	Chains [][32]byte // List of chains that the Group manages
	Signatories []Signator
}

//  Need to be able to find a Group for a chain
//  The group is managed by the signatories in the Group


// SignatorState
// Holds the signature groups.
type SignatorState struct {
	Groups []Group
}

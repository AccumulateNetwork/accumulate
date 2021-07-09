package pmt

// Entry
// We only have two node types, a Node that builds the Patricia Tree, and
// a Value that holds the values at the leaves.
type Entry interface {
	T() bool         // Returns the type of entry
	GetHash() []byte // Returns the Hash for the entry
}

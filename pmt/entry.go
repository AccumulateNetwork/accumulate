package pmt

// Entry
// We only have two node types, a Node that builds the Patricia Tree, and
// a Value that holds the values at the leaves.
type Entry interface {
	T() bool                      // Returns the type of entry
	GetHash() []byte              // Returns the Hash for the entry
	Marshal() []byte              // Serialize the state of the Node or Value
	UnMarshal(data []byte) []byte // Unmarshal the state into the Node or Value
	Equal(entry Entry) bool       // Return Entry == entry
}

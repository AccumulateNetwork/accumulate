package dagbft

import "testing"

// TestServingAcceptsWhenNeverJoined: throwaway, #4366. A node that never
// joined has a nil nodeState (cmd/accumulated/run/dagbft.go:620-626), and
// serving() then returns nil — Submit proceeds. Nothing on the path asks
// whether this node is in the partition's committee.
func TestServingAcceptsWhenNeverJoined(t *testing.T) {
	s := &SubmitterService{} // nodeState nil: never joined
	if err := s.serving("Submit"); err != nil {
		t.Fatalf("serving refused a never-joined node: %v", err)
	}
	t.Log("serving() returned nil for a node with no join state and no committee membership")
}

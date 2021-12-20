package managed

import (
	"bytes"
	"fmt"
	"math"
)

// Node
// Holds the Left/Right hash combinations for Receipts
type Node struct {
	Right bool
	Hash  Hash
}

// Receipt
// Struct builds the Merkle Tree path component of a Merkle Tree Proof.
type Receipt struct {
	Element      Hash    // Hash for which we want a proof.
	ElementIndex int64   //
	Anchor       Hash    // Hash at the point where the Anchor was created.
	AnchorIndex  int64   //
	MDRoot       Hash    // Root expected once all nodes are applied
	Nodes        []*Node // Apply these hashes to create an anchor

	// None of the following is persisted.
	height       int            // Track the height in the Merkle Tree during builds
	right        bool           // Track if the next element comes from the left or right
	manager      *MerkleManager // The Merkle Tree Manager from which we are building a receipt
	currentState *MerkleState   // The state we are walking through as we build the receipt
	anchorState  *MerkleState   // The state from which we will ultimately build the anchor
}

// AddANode
// Adds a hash step to the receipt. Takes two hashes, an option for the left
// path and an option for the right path, and the bool for which way to go.
// If the direction is known, both hashes do not have to be provided.
func (r *Receipt) AddANode(left, right Hash) {
	node := new(Node)
	r.Nodes = append(r.Nodes, node)
	node.Right = r.right
	if r.right {
		AM.Chk(right)
		node.Hash = right.Copy()
	} else {
		AM.Chk(left)
		node.Hash = left.Copy()
	}
	r.right = false
}

func (r *Receipt) AddAHash(v1 Hash) {
	original := v1.Copy()
	r.currentState.PadPending() // Pending always has to end in a nil to ensure we handle the "carry" case
	for j, v2 := range r.currentState.Pending {
		if v2 == nil {
			// If we find a nil spot, then we found where the hash will go.  To keep
			// the accounting square, we won't add it ourselves, but will let the Merkle Tree
			// library do the deed.  That keeps the Merkle Tree State square.
			r.currentState.AddToMerkleTree(original)
			if j == r.height { // If we combine with our proof height, the NEXT combination will be
				r.MDRoot = original
				r.right = true // from the right.
			}
			return
		}
		// If this is the creation of a higher derivative of object, put it in our path
		if j == r.height {
			r.AddANode(v2, v1)
			r.height++      // The Anchor will now move up one level in the Merkle Tree
			r.right = false // The Anchor hashes on the left are now added to the Anchor
		}
		// v1 becomes HashOf(pending[j]+v1)
		v1 = r.currentState.HashFunction(append(v2, v1...))
	}
	panic("failed to add the hash to the current state")
}

// ComputeDag
// Creates the Merkle Dag Root MDRoot
func (r *Receipt) ComputeDag() {
	var MDRoot Hash
	r.right = true

	for i, v := range r.currentState.Pending {
		if v == nil { // if v is nil, there is nothing to do regardless. Note i cannot == Height
			continue // because the previous code tracks Height such that there is always a value.
		}
		if MDRoot == nil { // Find the first non nil in pending
			MDRoot = AM.Chk(v)
			if i == r.height {
				r.right = false
			}
			continue
		}
		if i >= r.height { // At this point, we have a hash in the MDRoot, and we are
			r.AddANode(v, MDRoot)
			r.right = false
		}
		r.MDRoot = v.Combine(r.manager.MS.HashFunction, MDRoot)
	}
	// We are testing MDRoot, but it can't be nil
	if r.MDRoot == nil {
		panic("MDRoot was nil, and this should not be possible.")
	}
}

// String
// Convert the receipt to a string
func (r *Receipt) String() string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("\nElement      %x\n", r.Element))    // Element of proof
	b.WriteString(fmt.Sprintf("ElementIndex %d\n", r.ElementIndex)) // Element of proof
	b.WriteString(fmt.Sprintf("Anchor       %x\n", r.Anchor))       // Anchor point in the Merkle Tree
	b.WriteString(fmt.Sprintf("AnchorIndex  %d\n", r.AnchorIndex))  // Anchor point in the Merkle Tree
	b.WriteString(fmt.Sprintf("MDRoot       %x\n", r.MDRoot))       // MDRoot result of evaluating the receipt path
	working := r.Element                                            // Calculate the receipt path; for debugging print the
	for i, v := range r.Nodes {                                     // intermediate hashes
		r := "L"
		if v.Right {
			r = "R"
			working = Sha256(append(working[:], v.Hash[:]...))
		} else {
			working = Sha256(append(v.Hash[:], working[:]...))
		}
		b.WriteString(fmt.Sprintf(" %10d Apply %s %x working: %x \n", i, r, v.Hash, working))
	}
	return b.String()
}

// getNextState
// Return the state at th idx, or the last state if idx isn't on the chain
// Note the state returned is the one just before the target (because we may
// wish to add the entry that brings us to the target.
func getNextState(m *MerkleManager, limit, idx int64) (*MerkleState, int64) {
	if idx >= limit {
		s, _ := m.GetAnyState(limit)
		return s, limit
	}
	s, _ := m.GetAnyState(idx)
	return s, idx
}

// BuildReceipt
// Do the actual lift to build a receipt
func (r *Receipt) BuildReceipt() (err error) {

	nextMark := r.ElementIndex & ^r.manager.MarkMask + r.manager.MarkFreq - 2

	var idx int64
	for idx = r.ElementIndex; idx <= nextMark; idx++ { // Create the r upto the next mark
		r.currentState.Pad() // Pad the pending list with a nil entry. Trim() will remove it.
		switch {
		case idx == r.AnchorIndex:
			hash, _ := r.manager.Get(idx)
			r.AddAHash(hash)
			r.ComputeDag()
			return nil
		default:
			hash, _ := r.manager.Get(idx)
			r.AddAHash(hash)
		}
	}

	r.currentState, idx = getNextState(r.manager, r.AnchorIndex, nextMark+1)

	for {
		switch {
		case idx == r.AnchorIndex:
			r.currentState, idx = getNextState(r.manager, r.AnchorIndex, nextMark)
			r.ComputeDag()
			return nil
		case idx > r.AnchorIndex:
			r.ComputeDag()
			return nil
		default:
			_, hRight, err := r.manager.GetIntermediate(idx, int64(r.height))
			if err != nil {
				return err
			}
			AM.Chk(hRight)
			r.AddANode(nil, hRight)
			r.height++
			nextMark = nextMark + int64(math.Pow(2, float64(r.height)))
			idx = nextMark
		}
	}
}

// Validate
// Take a receipt and validate that the element hash progresses to the
// Merkle Dag Root hash (MDRoot) in the receipt
func (r Receipt) Validate() bool {
	MDRoot := r.Element // To begin with, we start with the object as the MDRoot
	// Now apply all the path hashes to the MDRoot
	for _, node := range r.Nodes {
		// Need a [32]byte to slice
		hash := node.Hash
		if node.Right {
			// If this hash comes from the right, apply it that way
			MDRoot = MDRoot.Combine(Sha256, hash)
		} else {
			// If this hash comes from the left, apply it that way
			MDRoot = hash.Combine(Sha256, MDRoot)
		}
	}
	// In the end, MDRoot should be the same hash the receipt expects.
	return MDRoot.Equal(r.MDRoot)
}

// Copy
// Make a copy of this receipt
func (r Receipt) Copy() *Receipt {
	nr := new(Receipt)
	nr.Element = append([]byte{}, r.Element...)
	nr.Anchor = append([]byte{}, r.Anchor...)
	nr.MDRoot = append([]byte{}, r.MDRoot...)
	for _, n := range r.Nodes {
		node := Node{Right: n.Right, Hash: append([]byte{}, n.Hash...)}
		nr.Nodes = append(nr.Nodes, &node)
	}
	return nr
}

// Combine
// Take a 2nd receipt, attach it to a root receipt, and return the resulting
// receipt.  The idea is that if this receipt is anchored into another chain,
// Then we can create a receipt that proves the element in this receipt all
// the way down to an anchor in the root receipt.
// Note that both this receipt and the root receipt are expected to be good.
func (r *Receipt) Combine(rm *Receipt) (*Receipt, error) {
	if !bytes.Equal(r.MDRoot, rm.Element) {
		return nil, fmt.Errorf("receipts cannot be combined. "+
			"anchor %x doesn't match root merkle tree %x", r.Anchor, rm.Element)
	}
	nr := r.Copy()
	nr.MDRoot = rm.MDRoot
	for _, n := range rm.Nodes {
		nr.Nodes = append(nr.Nodes, n)
	}
	return nr, nil
}

// PrintState
// Print the state at this time.
func PrintState(title string, height int, state1 *MerkleState, hash1 Hash) {
	fmt.Println("===============", title, "height: ", height, " ===============")
	if state1 != nil {
		fmt.Printf("%s\n", state1.String())
		if hash1 != nil {
			state1.Trim()
			hash := state1.Pending[len(state1.Pending)-1]
			L := Sha256(append(hash, hash1...))
			R := Sha256(append(hash1, hash...))
			fmt.Printf("AddHash %10x  L %10x R %10x\n", hash1, L, R)
		}
	}
}

// GetReceipt
// Given a merkle tree and two elements, produce a proof that the element was used to derive the DAG at the anchor
// Note that the element must be added to the Merkle Tree before the anchor, but the anchor can be any element
// after the element, or even the element itself.
func GetReceipt(manager *MerkleManager, element Hash, anchor Hash) (r *Receipt, err error) {
	// Allocate r, the receipt we are building and record our element
	r = new(Receipt)    // Allocate a r
	r.Element = element // Add the element to the r
	r.Anchor = anchor   // Add the anchor hash to the r
	r.manager = manager
	if r.ElementIndex, err = r.manager.GetElementIndex(element); err != nil {
		return nil, err
	}
	if r.AnchorIndex, err = r.manager.GetElementIndex(anchor); err != nil {
		return nil, err
	}

	if r.ElementIndex > r.AnchorIndex ||
		r.ElementIndex < 0 ||
		r.ElementIndex > r.manager.GetElementCount() { // The element must be at the anchorIndex or before
		return nil, fmt.Errorf("invalid indexes for the element %d and anchor %d", r.ElementIndex, r.AnchorIndex)
	}

	if r.ElementIndex == 0 && r.AnchorIndex == 0 { // If this is the first element in the Merkle Tree, we are already done.
		r.MDRoot = element // A Merkle Tree of one element has a root of the element itself.
		return r, nil      // And we are done!
	}

	if r.anchorState, err = r.manager.GetAnyState(r.AnchorIndex); err != nil {
		return nil, err
	}
	if r.ElementIndex == 0 {
		r.currentState = new(MerkleState)
	} else if r.currentState, err = r.manager.GetAnyState(r.ElementIndex - 1); err != nil { // Need to combine our element
		return nil, err // with the currentState, so the currentState we need is the one before the element.
	}
	r.currentState.InitSha256()

	if err := r.BuildReceipt(); err != nil {
		return nil, err
	}
	return r, nil
}

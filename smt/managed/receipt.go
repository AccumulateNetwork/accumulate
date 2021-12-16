package managed

import (
	"bytes"
	"fmt"
	"math"
)

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
}

func (r *Receipt) AddAHash(
	m *MerkleManager,
	CurrentState *MerkleState,
	Height int,
	Right bool,
	v1 Hash,
) (
	height int, right bool) {
	original := v1.Copy()
	CurrentState.PadPending() // Pending always has to end in a nil to ensure we handle the "carry" case
	for j, v2 := range CurrentState.Pending {
		if v2 == nil {
			// If we find a nil spot, then we found where the hash will go.  To keep
			// the accounting square, we won't add it ourselves, but will let the Merkle Tree
			// library do the deed.  That keeps the Merkle Tree State square.
			CurrentState.AddToMerkleTree(original)
			if j == Height { // If we combine with our proof height, the NEXT combination will be
				Right = true // from the right.
			}

			return Height, Right
		}
		// If this is the creation of a higher derivative of object, put it in our path
		if j == Height {
			node := new(Node)
			r.Nodes = append(r.Nodes, node)
			node.Right = Right
			if Right {
				node.Hash = v1.Copy()
			} else {
				node.Hash = v2.Copy()
			}
			//fmt.Printf("Height %3d %x\n", Height, node.Hash)
			// v1 becomes HashOf(pending[j]+v1)
			Height++      // The Anchor will now move up one level in the Merkle Tree
			Right = false // The Anchor hashes on the left are now added to the Anchor
		}
		// v1 becomes HashOf(pending[j]+v1)
		v1 = CurrentState.HashFunction(append(v2, v1...))
	}
	panic("failed to add the hash to the current state")
}

func (r *Receipt) ComputeDag(currentState *MerkleState, height int, right bool) {
	var MDRoot Hash
	for i, v := range currentState.Pending {
		if v == nil { // if v is nil, there is nothing to do regardless. Note i cannot == Height
			continue // because the previous code tracks Height such that there is always a value.
		}
		if MDRoot == nil { // Find the first non nil in pending
			MDRoot = v.Copy() // Copy the MDRoot
			if i == height {  // Note that there is no way this code does not
				right = false //      execute before the following code that adds
			} else { //               applyHash records to the receipt
				right = true
			}
			continue
		}
		if i >= height { // At this point, we have a hash in the MDRoot, and we are
			node := new(Node) // combining hashes as we go.
			r.Nodes = append(r.Nodes, node)
			node.Right = right // We record if the proof path is in the MDRoot
			if right {         // or in the pending array
				node.Hash = MDRoot
			} else {
				node.Hash = v
			}
			// Note once the object derivative is in MDRoot, it never leaves. All combining
			right = false // then comes from the left
			// Combine all the pending hash values into the MDRoot
			MDRoot = v.Combine(currentState.HashFunction, MDRoot)
		}
	}
	// We are testing MDRoot, but it can't be nil
	if MDRoot == nil {
		panic("MDRoot was nil, and this should not be possible.")
	}
	// Put the resulting MDRoot into the receipt
	r.MDRoot = MDRoot
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
	if idx > limit {
		s, _ := m.GetAnyState(limit - 1)
		return s, limit
	}
	s, _ := m.GetAnyState(idx - 1)
	return s, idx
}

// GetReceipt
// Given a merkle tree and two elements, produce a proof that the element was used to derive the DAG at the anchor
// Note that the element must be added to the Merkle Tree before the anchor, but the anchor can be any element
// after the element, or even the element itself.
func GetReceipt(manager *MerkleManager, element Hash, anchor Hash) *Receipt {
	elementIndex, e1 := manager.GetElementIndex(element[:]) // Get the index of the element in the Merkle Tree; -1 if not found
	anchorIndex, e2 := manager.GetElementIndex(anchor[:])   // Get the index of the anchor in the Merkle Tree; -1 if not found
	if e1 != nil || e2 != nil {                             // Both the element and the anchor must be in the Merkle Tree
		return nil
	}
	if elementIndex > anchorIndex ||
		elementIndex < 0 ||
		elementIndex > manager.GetElementCount() { // The element must be at the anchorIndex or before
		return nil
	}

	// Allocate our receipt and record our element
	receipt := new(Receipt)             // Allocate a receipt
	receipt.Element = element           // Add the element to the receipt
	receipt.Anchor = anchor             // Add the anchor hash to the receipt
	receipt.ElementIndex = elementIndex // Merkle Tree indexes
	receipt.AnchorIndex = anchorIndex   // Merkle Tree indexes

	if elementIndex == 0 && anchorIndex == 0 { //      If this is the first element in the Merkle Tree, we are already done.
		receipt.MDRoot = element // A Merkle Tree of one element has a root of the element itself.
		return receipt           // And we are done!
	}

	height := 0
	anchorState, _ := manager.GetAnyState(anchorIndex)
	currentState, err := manager.GetAnyState(elementIndex - 1)
	if err != nil {
		currentState = new(MerkleState)
		currentState.InitSha256()
	}
	nextMark := elementIndex & ^manager.MarkMask + manager.MarkFreq
	Right := true           // Even hashes combine from the right (Guess Right)
	if elementIndex&1 > 0 { // Odd hashes combine from the left.
		Right = false //        Guessed wrong, so set to left.
	}

	// The CurrentState is always one element behind, because we need to add
	// the elements to the receipt
	var idx int64
	for idx = elementIndex; idx < nextMark-1; idx++ { // Create the receipt upto the next mark
		currentState.Pad() // Pad the pending list with a nil entry. Trim() will remove it.
		switch {
		case idx == anchorIndex:
			height, Right = receipt.AddAHash(manager, currentState, height, Right, anchor)
			receipt.ComputeDag(currentState, height, Right)
			return receipt
		default:
			hash, _ := manager.Get(idx)
			height, Right = receipt.AddAHash(manager, currentState, height, Right, hash)
		}
	}

	currentState, idx = getNextState(manager, anchorIndex, nextMark-1)

	for {
		switch {
		case idx == anchorIndex:

			hash, _ := manager.Get(idx)
			height, Right = receipt.AddAHash(manager, currentState, height, Right, hash)
			receipt.ComputeDag(anchorState, height, Right)
			return receipt
		case idx > anchorIndex:
			receipt.ComputeDag(anchorState, height, Right)
			return receipt
		default:
			hash, _ := manager.Get(idx)
			height, Right = receipt.AddAHash(manager, currentState, height, Right, hash)
			nextMark = nextMark + int64(math.Pow(2, float64(height)))
			currentState, idx = getNextState(manager, anchorIndex, nextMark-1)
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

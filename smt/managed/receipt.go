package managed

import (
	"bytes"
	"fmt"
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

// GetElementState
// Looks through the Merkle Tree, and finds the state just before the element has been added to the
// Merkle Tree, and the height in the Pending list of the derivative of the element.
// Returns a nil for the merkleState if the element is not in the Merkle Tree
func GetElementState(
	manager *MerkleManager, //                       Parameters
	element Hash,
	elementIndex int64,
) (
	currentState, nextMark *MerkleState, //          Return values
	height int,
	nextMarkIndex int64) {
	// currentState is the state we are going to modify until it is the state we want
	previousMarkIndex := elementIndex & ^manager.MarkMask // Find the mark previous to element (current if at mark)
	if previousMarkIndex == 0 {                           // Get that state
		currentState = new(MerkleState)
	} else {
		currentState = manager.GetState(previousMarkIndex - 1)
	}
	currentState.InitSha256()                                //   Use sha256
	nextMarkIndex = previousMarkIndex + manager.MarkFreq - 1 //   Next mark has the list of elements to add
	nextMark = manager.GetState(nextMarkIndex)               //   Get that NextMark state. Now bracket element, search
	if nextMark == nil {                                     //   If there is no state at the next mark,
		nextMark = manager.MS.Copy() //                         then just use the last state of the Merkle Tree
	}
	for i, v1 := range nextMark.HashList { //                Go through the pending elements
		nextMarkIndex = int64(i)         //                   Return location in this HashList for next step
		currentState.AddToMerkleTree(v1) //                   Otherwise, just add value to merkle tree and keep looking

		if bytes.Equal(v1, element) { //                   If the element is found
			return currentState, nextMark, height, int64(i) // Return location so following code can reuse it
		}

	}
	panic("the element must be a member of the NextMark.HashList")
}

// AdvanceToMark
// Advance the currentState (from any element) to the next Mark.
// Receipt is updated, where our proof is expected to be at height in the currentState
// If advancing to the next mark would take the currentState past the AnchorState, then
// AdvanceToMark only advances to the anchorState and returns true.
//
// Returns true (that currentState is now the anchorState) or false
func (r *Receipt) AdvanceToMark(
	manager *MerkleManager,
	currentState, nextMark, anchorState *MerkleState,
	height int,
	markIndex int64,
) (
	atAnchor bool,
	newHeight int) {

	// Add all the hashes that remain in the HashList of the next Mark to the current state.
	// The result will be a current state that is at the next mark.
	for i, v := range nextMark.HashList[markIndex:] {
		if currentState.Count == anchorState.Count { //           Sort to see if reached the anchor state
			return true, height //                                 If at the anchor, signal currentState is at the anchor
		}
		if i == 0 && //                                           If adding the first hash of the hash list
			currentState.Count&1 == 1 { //                         And at an odd number of elements in the Merkle Tree
			height = r.AddAHash(manager, currentState, height, false, v) // In that case, the hash will come from the left
		} else { //                                                    Otherwise
			height = r.AddAHash(manager, currentState, height, true, v) //  the hash will come from the right
		}
	}
	// If reaching the next mark doesn't reach the anchor state, signal that the search continues
	return false, height
}

// AdvanceMarkToMark
// Once the currentState is at a Mark, it can be advanced directly to the next Mark.
// The next Mark is a mark at a 2^n step forward through the elements. These states
// can be combined directly without processing all the intermediate elements.
//
// Returns true (that the currentState is now the anchorState) or false
func (r *Receipt) AdvanceMarkToMark(manager *MerkleManager, anchorState, currentState *MerkleState, height int) (
	atAnchor bool, newHeight int) {
	// If the anchorState is between the currentState and the next MarkState, then none of the pending
	// entries in the AnchorState will contribute to the receipt, so we can build our proof from the
	// anchorStep directly.
	if currentState.Count+manager.MarkFreq >= anchorState.Count {
		currentState = anchorState.Copy()
		return true, height
	}
	// Set the currentState to the state just before the next mark.  None of the intermediate elements
	// will contribute to the receipt, so they don't heave to be evaluated.  But the next element
	// right before the mark does contribute.
	currentState = manager.GetState(currentState.Count + manager.MarkFreq - 1)
	// markNext is the next element to be added to the Merkle Tree.  It WILL carry into height, so
	// we have to use AddAHash to add its contribution to the receipt.
	markNext := manager.GetNext(currentState.Count + manager.MarkFreq - 1) // Get the the next element
	height = r.AddAHash(manager, currentState, height, true, markNext)     // Record its impact on the receipt
	// Handle the special case where the Mark we jump to == the AnchorState
	if currentState.Count == anchorState.Count {
		return true, height
	}
	return false, height
}

func (r *Receipt) AddAHash(
	m *MerkleManager,
	CurrentState *MerkleState,
	Height int,
	Right bool,
	v1 Hash,
) (
	height int) {
	original := v1
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

			cnt := CurrentState.Count
			s, _ := m.GetAnyState(cnt - 1)
			if !s.Equal(CurrentState) {
				fmt.Printf("AddAHash\n  %s\n%s\n", s.String(), CurrentState.String())
				panic("Not right!")
			}

			return Height
		}
		// If this is the creation of a higher derivative of object, put it in our path
		if j == Height {
			node := new(Node)
			r.Nodes = append(r.Nodes, node)
			node.Right = Right
			if Right {
				node.Hash = v1
			} else {
				node.Hash = v2
			}
			//fmt.Printf("Height %3d %x\n", Height, node.Hash)
			// v1 becomes HashOf(pending[j]+v1)
			Height++      // The Anchor will now move up one level in the Merkle Tree
			Right = false // The Anchor hashes on the left are now added to the Anchor
		}
		// v1 becomes HashOf(pending[j]+v1)
		v1 = CurrentState.HashFunction(append(v2[:], v1[:]...))
	}
	panic("failed to add the hash to the current state")
}

func (r *Receipt) ComputeDag(currentState *MerkleState, height int, right bool) {
	fmt.Printf("ComputeDag State\n %s\n", currentState.String())
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
	b.WriteString(fmt.Sprintf("\nObject      %x\n", r.Element))    // Element of proof
	b.WriteString(fmt.Sprintf("ObjectIndex %d\n", r.ElementIndex)) // Element of proof
	b.WriteString(fmt.Sprintf("Anchor      %x\n", r.Anchor))       // Anchor point in the Merkle Tree
	b.WriteString(fmt.Sprintf("AnchorIndex %d\n", r.AnchorIndex))  // Anchor point in the Merkle Tree
	b.WriteString(fmt.Sprintf("MDRoot      %x\n", r.MDRoot))       // MDRoot result of evaluating the receipt path
	working := r.Element                                           // Calculate the receipt path; for debugging print the
	for i, v := range r.Nodes {                                    // intermediate hashes
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
	if elementIndex > anchorIndex { // The element must be at the anchorIndex or before
		return nil
	}

	// Get the merkle state at the point where the anchor has been added to the Merkle tree.
	// Note that getting the ElementState occurs one state BEFORE the anchor was added, so
	// we will have to add the anchor
	anchorState, _ := manager.GetAnyState(anchorIndex)

	// Get the state just before the element is added to the merkle tree
	currentState, nextMark, height, nextMarkIndex := GetElementState(manager, element, elementIndex)
	// Allocate our receipt and record our element
	receipt := new(Receipt)             // Allocate a receipt
	receipt.Element = element           // Add the element to the receipt
	receipt.Anchor = anchor             // Add the anchor hash to the receipt
	receipt.ElementIndex = elementIndex // Merkle Tree indexes
	receipt.AnchorIndex = anchorIndex   // Merkle Tree indexes
	// If the current state is the anchor state, then we are done.
	if currentState.Count == anchorState.Count {
		receipt.ComputeDag(anchorState, height, true) // Compute the DAG and return
		return receipt
	}

	var MDRoot bool // Advance the current state to the first Mark at or past the current State
	MDRoot, height = receipt.AdvanceToMark(manager, currentState, nextMark, anchorState, height, nextMarkIndex)
	if MDRoot { // If we advanced the current state and the result is the Anchor State, then we are done
		receipt.ComputeDag(anchorState, height, true)
		return receipt
	}

	cnt := currentState.Count
	s, _ := manager.GetAnyState(cnt - 1)
	if !s.Equal(currentState) {
		panic("Not right!")
	}

	for { // Push the Current State from mark to mark, at ever higher powers of 2 until we reach the anchor state
		if MDRoot, height = receipt.AdvanceMarkToMark(manager, currentState, anchorState, height); MDRoot == true {
			receipt.ComputeDag(currentState, height, true) // We reach the anchor state, we are done
			return receipt
		}
	}
}

// Receipt
// Take a receipt and validate that the
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
		fmt.Println("r\n\n", r.String(), "rm\n", rm.String())
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

package smt

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
)

type Node struct {
	Right bool
	Hash  [32]byte
}

// Receipt
// Struct builds the Merkle Tree path component of a Merkle Tree Proof.
type Receipt struct {
	Element Hash    // Hash for which we want a proof.
	Anchor  Hash    // Directory Block Height of the Object
	Nodes   []*Node // Apply these hashes to create an anchor
}

// GetElementState
// Looks through the Merkle Tree, and finds the state just before the element has been added to the
// Merkle Tree, and the height in the Pending list of the derivative of the element.
// Returns a nil for the merkleState if the element is not in the Merkle Tree
func GetElementState(
	manager *MerkleManager, //                       Parameters
	element Hash,
) (
	CurrentState, nextMark *MerkleState, //          Return values
	height int,
	nextMarkIndex int) {

	elementIndex := manager.GetIndex(element[:])
	if elementIndex == -1 {
		return nil, nil, -1, -1
	}
	// currentState is the state we are going to modify until it is the state we want
	currentIndex := elementIndex & manager.MarkMask  // Find the mark previous to element
	currentState := manager.GetState(currentIndex)   // Get that state
	currentState.InitSha256()                        // We are using sha256
	NextMarkIndex := currentIndex + manager.MarkFreq // The next mark has the list of elements to add including ours
	NextMark := manager.GetState(NextMarkIndex)      // Get that NextMark state. Now that we bracket element, search
	for i, v1 := range NextMark.HashList {           // Go through the pending elements
		nextMarkIndex = i  //                             Return where we are in this HashList for next step
		if v1 == element { //                                  If we have found the element
			return currentState, nextMark, height, i //   Return where we are so following code can reuse it
		}
		currentState.AddToMerkleTree(v1) // Otherwise just add the value to the merkle tree and keep looking
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
	height,
	markIndex int,
) (
	atAnchor bool,
	newHeight int) {

	for _, v := range nextMark.HashList[markIndex:] {
		if currentState.Count == anchorState.Count {
			return true, height
		}
		height = r.AddAHash(currentState, height, true, v)
		currentState.AddToMerkleTree(v)
	}
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
	// Calculate the number of elements from the currentState at height to the next Mark State
	markStep := int64(math.Pow(2, float64(height)))
	// If the anchorState is between the currentState and the next MarkState, then none of the pending
	// entries in the AnchorState will contribute to the receipt, so we can build our proof from the
	// anchorStep directly.
	if currentState.Count+markStep > anchorState.Count {
		currentState = anchorState
		return true, height
	}
	// Set the currentState to the state just before the next mark.  None of the intermediate elements
	// will contribute to the receipt, so they don't heave to be evaluated.  But the next element
	// right before the mark does contribute.
	currentState = manager.GetState(currentState.Count + markStep - 1)
	// markNext is the next element to be added to the Merkle Tree.  It WILL carry into height, so
	// we have to use AddAHash to add its contribution to the receipt.
	markNext := manager.GetNext(currentState.Count + markStep - 1) // Get the the next element
	height = r.AddAHash(currentState, height, true, *markNext)     // Record its impact on the receipt
	currentState.AddToMerkleTree(*markNext)                        // As always, actually update the current state
	// Handle the special case where the Mark we jump to == the AnchorState
	if currentState.Count == anchorState.Count {
		return true, height
	}
	return false, height
}

func (r *Receipt) AddAHash(
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
			return height
		}
		// If this is the creation of a higher derivative of object, put it in our path
		if j == Height {
			node := new(Node)
			r.Nodes = append(r.Nodes, node)
			node.Right = Right
			if Right {
				node.Hash = v1
			} else {
				node.Hash = *v2
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

func (r *Receipt) ComputeDag(manager *MerkleManager, currentState *MerkleState, height int, right bool) {
	var anchor *Hash
	for i, v := range currentState.Pending {
		if v == nil { // if v is nil, there is nothing to do regardless. Note i cannot == Height
			continue // because the previous code tracks Height such that there is always a value.
		}
		if anchor == nil { // Find the first non nil in pending
			anchor = new(Hash) // Make an anchor
			*anchor = *v       // copy over the value of v (which is never nil
			if i == height {   // Note that there is no way this code does not
				right = false //      execute before the following code that adds
			} else { //               applyHash records to the receipt
				right = true
			}
			continue
		}
		if i >= height { // At this point, we have a hash in the anchor, and we are
			node := new(Node) // combining hashes as we go.
			r.Nodes = append(r.Nodes, node)
			node.Right = right // We record if the proof path is in the anchor
			if right {         // or in the pending array
				node.Hash = *anchor
			} else {
				node.Hash = *v
			}
			// Note once the object derivative is in anchor, it never leaves. All combining
			right = false // then comes from the left
			// Combine all the pending hash values into the anchor
			hash := currentState.HashFunction(append((*v)[:], (*anchor)[:]...))
			anchor = &hash
		}
	}
	// We are testing anchor, but it can't be nil
	if anchor == nil {
		panic("anchor was nil, and this should not be possible.")
	}
	// Put the resulting anchor into the receipt
	r.Anchor = *anchor
}

// String
// Convert the receipt to a string
func (r *Receipt) String() string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("\nObject %x\n", r.Element)) // Element of proof
	b.WriteString(fmt.Sprintf("Anchor %x\n", r.Anchor))    // Anchor result of evaluating the receipt path
	working := r.Element                                   // Calculate the receipt path; for debugging print the
	for i, v := range r.Nodes {                            // intermediate hashes
		r := "L"
		if v.Right {
			r = "R"
			working = sha256.Sum256(append(working[:], v.Hash[:]...))
		} else {
			working = sha256.Sum256(append(v.Hash[:], working[:]...))
		}
		b.WriteString(fmt.Sprintf(" %10d Apply %s %x working: %x \n", i, r, v.Hash, working))
	}
	return b.String()
}

func GetReceipt(manager *MerkleManager, element Hash, anchor Hash) *Receipt {
	elementIndex := manager.GetIndex(element[:]) // Get the index of the element in the Merkle Tree; -1 if not found
	anchorIndex := manager.GetIndex(anchor[:])   // Get the index of the anchor in the Merkle Tree; -1 if not found
	if elementIndex == -1 || anchorIndex == -1 { // Both the element and the anchor must be in the Merkle Tree
		return nil
	}
	if elementIndex > anchorIndex { // The element must be at the anchorIndex or before
		return nil
	}
	// element and anchor have indexes, so these calls can't fail
	// Get a set of states that will start our receipt building
	currentState, nextMark, height, nextMarkIndex := GetElementState(manager, element)
	anchorState, _, _, _ := GetElementState(manager, element) // Get the state at the anchor

	receipt := new(Receipt)   // Allocate a receipt
	receipt.Element = element // Add the element to the receipt

	height = receipt.AddAHash(currentState, height, false, element) // Add the element to the receipt
	currentState.AddToMerkleTree(element)                           //  and add the element to the MerkleTree
	if currentState.Count == anchorState.Count {
		return nil //TODO: we are done here
	}
	var atAnchor bool
	atAnchor, height = receipt.AdvanceToMark(manager, currentState, nextMark, anchorState, height, nextMarkIndex)
	if atAnchor {
		return nil //TODO: we are done
	}
	for {
		if atAnchor, height = receipt.AdvanceMarkToMark(manager, currentState, anchorState, height); atAnchor == true {
			return nil //TODO: we are done
		}
	}
}

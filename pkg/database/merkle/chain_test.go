// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
)

func begin() database.Store {
	store := memory.New(nil)
	txn := store.Begin(nil, true)
	return keyvalue.RecordStore{Store: txn}
}

func testChain(store record.Store, markPower int64, key ...interface{}) *Chain {
	return NewChain(nil, store, record.NewKey(key...), markPower, ChainTypeUnknown, "chain")
}

func TestMerkleManager_GetChainState(t *testing.T) {
	const numTests = 100
	var randHash common.RandHash
	store := begin()
	m := testChain(store, 8, "try")
	err := m.Head().Put(new(State))
	require.NoError(t, err, "should be able to write to the chain head")
	_, err = m.Head().Get()
	require.NoError(t, err, "should be able to read the chain head")

	for i := 0; i < numTests; i++ {
		require.NoError(t, m.AddEntry(randHash.Next(), false))
		head, err := m.Head().Get()
		require.NoError(t, err)
		mState, err := head.marshal()
		require.NoError(t, err, "must be able to marshal a State")
		ms := new(State)
		err = ms.unmarshal(mState)
		require.NoError(t, err, "must be able to unmarshal a State")
		require.True(t, ms.Equal(head), " should get the same state back")
		cState, e2 := m.Head().Get()
		require.NoErrorf(t, e2, "chain should always have a chain state %d", i)
		require.Truef(t, cState.Equal(head), "should be the last state of the chain written (%d)", i)
	}
}

func TestMerkleManager_GetAnyState(t *testing.T) {
	const testnum = 100
	var randHash common.RandHash
	store := begin()
	m := testChain(store, 2, "try")
	var States []*State
	for i := 0; i < testnum; i++ {
		require.NoError(t, m.AddEntry(randHash.Next(), false))
		head, err := m.Head().Get()
		require.NoError(t, err)
		States = append(States, head.Copy())
		println(PrintMerkleState(States[i]))
	}
	for i := int64(0); i < testnum; i++ {
		state, err := m.StateAt(i)
		if err != nil {
			state, err = m.StateAt(i)
		}
		require.Truef(t, state.Count == i+1, "state count %d does not match %d", state.Count, i)
		require.NoErrorf(t, err, "%d all elements should have a state: %v", i, err)
		if !state.Equal(States[i]) {
			fmt.Println("i=", i)
			fmt.Println("=============", PrintMerkleState(state))
			fmt.Println("-------------", PrintMerkleState(States[i]))
		}
		require.Truef(t, state.Equal(States[i]), "All states should be equal height %d", i)
	}
}

func TestIndexing2(t *testing.T) {
	const testlen = 1024

	store := begin()

	Chain := sha256.Sum256([]byte("RedWagon/ACME_tokens"))
	BlkIdx := Chain
	BlkIdx[30] += 2

	MM1 := testChain(store, 8, Chain[:])
	for i := 0; i < testlen; i++ {
		data := []byte(fmt.Sprintf("data %d", i))
		dataHash := sha256.Sum256(data)
		require.NoError(t, MM1.AddEntry(dataHash[:], false))
		di, e := MM1.ElementIndex(dataHash[:]).Get()
		if e != nil {
			t.Fatalf("error")
		}
		if di != uint64(i) {
			t.Fatalf("didn't get the right index. got %d expected %d", di, i)
		}
		d, e2 := MM1.Element(uint64(i)).Get()
		if e2 != nil || !bytes.Equal(d, dataHash[:]) {
			t.Fatalf("didn't get the data back. got %d expected %d", d, data)
		}
	}

}

func TestMerkleManager(t *testing.T) {

	const testLen = 1024

	store := begin()

	MarkPower := int64(2)
	MarkFreq := int64(math.Pow(2, float64(MarkPower)))
	MarkMask := MarkFreq - 1

	// Set up a MM1 that uses a MarkPower of 2
	MM1 := testChain(store, MarkPower, "try")
	// if MarkPower != MM1.MarkPower ||
	// 	MarkFreq != MM1.MarkFreq ||
	// 	MarkMask != MM1.MarkMask {
	// 	t.Fatal("Marks were not correctly computed")
	// }

	// Fill the Merkle Tree with a few hashes
	hash := sha256.Sum256([]byte("start"))
	for i := 0; i < testLen; i++ {
		require.NoError(t, MM1.AddEntry(hash[:], false))
		hash = sha256.Sum256(hash[:])
	}

	head, err := MM1.Head().Get()
	require.NoError(t, err)
	if head.Count != testLen {
		t.Fatal("added elements in merkle tree don't match the number we added")
	}

	// Sort the Indexing
	for i := int64(0); i < testLen; i++ {
		ms := MM1.getState(i)
		if i&MarkMask == MarkFreq-1 {
			if ms == nil {
				t.Fatal("should have a state at Mark point - 1 at ", i)
			}
		} else if i&MarkMask == 0 {
			if ms != nil && i != 0 {
				t.Fatal("should not have a state at Mark point at ", i)
			}
		} else {
			if ms != nil {
				t.Fatal("should not have a state outside Mark points at ", i)
			}
		}

	}
}

func GenerateTestData(prt bool) [10][][]byte {
	spaces := func(i int) {
		n := int(math.Pow(2, float64(i+1))) - 1
		for i := 0; i < n; i++ {
			print("          ")
		}
	}
	var rp common.RandHash
	var hashes [10][][]byte
	row := 0
	for i := 0; i < 20; i++ {
		v := rp.Next()
		hashes[row] = append(hashes[row], v)
	}

	if !prt {
		return hashes
	}

	// Print the first row
	for _, v := range hashes[0] {
		fmt.Printf("%3v ", v[:2]) //nolint:rangevarref
	}
	fmt.Println()
	for len(hashes[row]) > 1 {
		for i := 0; i+1 < len(hashes[row]); i += 2 {
			v := combineHashes(hashes[row][i], hashes[row][i+1])
			hashes[row+1] = append(hashes[row+1], v)
			spaces(row)
			fmt.Printf("%3v ", v[:2])
		}
		row++
		println()
	}
	ms := new(State)
	for _, v := range hashes[0] {
		ms.AddEntry(v)
		mdr := ms.Anchor()
		fmt.Printf("%3v ", mdr[:2])
	}
	println("\n")

	for _, v := range hashes[0] {
		fmt.Printf("%x  ", v[:4]) //nolint:rangevarref
	}
	fmt.Println()
	row = 0
	for len(hashes[row]) > 1 {
		for i := 0; i+1 < len(hashes[row]); i += 2 {
			v := combineHashes(hashes[row][i], hashes[row][i+1])
			hashes[row+1][(i+1)/2] = v
			spaces(row)
			fmt.Printf("%x  ", v[:4])
		}
		row++
		println()
	}
	ms = new(State)
	for _, v := range hashes[0] {
		ms.AddEntry(v)
		mdr := ms.Anchor()
		fmt.Printf("%x  ", mdr[:4])
	}
	println()

	return hashes
}

func TestMerkleManager_GetIntermediate(t *testing.T) {
	store := begin()
	m := testChain(store, 4)

	hashes := GenerateTestData(true)

	var r common.RandHash

	for col := int64(0); col < 20; col++ {
		require.NoError(t, m.AddEntry(r.NextList(), false))
		head, err := m.Head().Get()
		require.NoError(t, err)
		head.pad()
		if col&1 == 1 {
			s, _ := m.StateAt(col - 1)
			s.pad()
			for row := int64(1); s.Pending[row-1] != nil; row++ {
				left, right, err := m.getIntermediate(col, row)
				require.Nil(t, err, err)
				factor := int64(math.Pow(2, float64(row)))
				fmt.Printf("Row %d Col %d Left %x + Right %x == %x == Result %x\n",
					row, col, left[:4], right[:4],
					combineHashes(left, right)[:4],
					hashes[row][col/factor][:4])
				require.True(t, bytes.Equal(combineHashes(left, right), hashes[row][col/factor]), "should be equal")
			}
		}
	}

}

func TestMerkleManager_AddHash_Unique(t *testing.T) {
	var r common.RandHash
	hash := r.NextList()

	t.Run("true", func(t *testing.T) {
		store := begin()
		m := testChain(store, 4)
		head, err := m.Head().Get()
		require.NoError(t, err)

		require.NoError(t, m.AddEntry(hash, true))
		require.NoError(t, m.AddEntry(hash, true))
		require.Equal(t, int64(1), head.Count)
	})

	t.Run("false", func(t *testing.T) {
		store := begin()
		m := testChain(store, 4)
		head, err := m.Head().Get()
		require.NoError(t, err)

		require.NoError(t, m.AddEntry(hash, false))
		require.NoError(t, m.AddEntry(hash, false))
		require.Equal(t, int64(2), head.Count)
	})
}

// String
// convert the State to a human readable string
func PrintMerkleState(m *State) string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("%20s %d\n", "Count", m.Count))
	b.WriteString(fmt.Sprintf("%20s %d\n", "Pending[] length:", len(m.Pending)))
	for i, v := range m.Pending {
		vp := "nil"
		if v != nil {
			vp = fmt.Sprintf("%x", v)
		}
		b.WriteString(fmt.Sprintf("%20s [%3d] %s\n", "", i, vp))
	}
	b.WriteString(fmt.Sprintf("%20s %d\n", "HashList Length:", len(m.HashList)))
	for i, v := range m.HashList {
		vp := fmt.Sprintf("%x", v)
		b.WriteString(fmt.Sprintf("%20s [%3d] %s\n", "", i, vp))
	}
	return b.String()
}

func Cascade(c, hashList [][]byte, maxHeight int64) [][]byte {
	return cascade(c, hashList, maxHeight)
}

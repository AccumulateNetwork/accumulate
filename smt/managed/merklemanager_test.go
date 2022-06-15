package managed_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	. "gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func TestMerkleManager_GetChainState(t *testing.T) {
	const numTests = 100
	var randHash common.RandHash
	store := database.OpenInMemory(nil)
	storeTx := store.Begin(true)
	m, e2 := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 8)
	require.NoError(t, e2, "should be able to open a database")
	err := m.SetKey(storage.MakeKey("try"))
	require.NoError(t, err, "should be able to set base key")
	err = m.WriteChainHead()
	require.NoError(t, err, "should be able to write to the chain head")
	head, err := m.ReadChainHead()
	require.NoError(t, err, "should be able to read the chain head")
	require.True(t, head.Equal(m.MS), "chainstate should be loadable")

	for i := 0; i < numTests; i++ {
		require.NoError(t, m.AddHash(randHash.Next(), false))
		mState, err := m.MS.Marshal()
		require.NoError(t, err, "must be able to marshal a MerkleState")
		ms := new(MerkleState)
		err = ms.UnMarshal(mState)
		require.NoError(t, err, "must be able to unmarshal a MerkleState")
		require.True(t, ms.Equal(m.MS), " should get the same state back")
		cState, e2 := m.GetChainState()
		require.NoErrorf(t, e2, "chain should always have a chain state %d", i)
		require.Truef(t, cState.Equal(m.MS), "should be the last state of the chain written (%d)", i)
	}
}

func TestMerkleManager_GetAnyState(t *testing.T) {
	const testnum = 100
	var randHash common.RandHash
	store := database.OpenInMemory(nil)
	storeTx := store.Begin(true)
	m, e2 := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 2)
	require.NoError(t, e2, "should be able to open a database")
	var States []*MerkleState
	for i := 0; i < testnum; i++ {
		require.NoError(t, m.AddHash(randHash.Next(), false))
		States = append(States, m.MS.Copy())
		println(States[i].String())
	}
	for i := int64(0); i < testnum; i++ {
		state, err := m.GetAnyState(i)
		if err != nil {
			state, err = m.GetAnyState(i)
		}
		require.Truef(t, state.Count == i+1, "state count %d does not match %d", state.Count, i)
		require.NoErrorf(t, err, "%d all elements should have a state: %v", i, err)
		if !state.Equal(States[i]) {
			fmt.Println("i=", i)
			fmt.Println("=============", state.String())
			fmt.Println("-------------", States[i].String())
		}
		require.Truef(t, state.Equal(States[i]), "All states should be equal height %d", i)
	}
}

func TestIndexing2(t *testing.T) {
	const testlen = 1024

	store := database.OpenInMemory(nil)
	storeTx := store.Begin(true)

	Chain := sha256.Sum256([]byte("RedWagon/ACME_tokens"))
	BlkIdx := Chain
	BlkIdx[30] += 2

	MM1, err := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 8)
	if err != nil {
		t.Fatal("didn't create a Merkle Manager")
	}

	if err := MM1.SetKey(storage.MakeKey(Chain[:])); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < testlen; i++ {
		data := []byte(fmt.Sprintf("data %d", i))
		dataHash := sha256.Sum256(data)
		require.NoError(t, MM1.AddHash(dataHash[:], false))
		di, e := MM1.Manager.Int(storage.MakeKey(Chain, "ElementIndex", dataHash)).Get()
		if e != nil {
			t.Fatalf("error")
		}
		if di != int64(i) {
			t.Fatalf("didn't get the right index. got %d expected %d", di, i)
		}
		d, e2 := MM1.Manager.Hash(storage.MakeKey(Chain, "Element", i)).Get()
		if e2 != nil || !bytes.Equal(d, dataHash[:]) {
			t.Fatalf("didn't get the data back. got %d expected %d", d, data)
		}
	}

}

func TestMerkleManager(t *testing.T) {

	const testLen = 1024

	store := database.OpenInMemory(nil)
	storeTx := store.Begin(true)

	MarkPower := int64(2)
	MarkFreq := int64(math.Pow(2, float64(MarkPower)))
	MarkMask := MarkFreq - 1

	// Set up a MM1 that uses a MarkPower of 2
	MM1, err := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, MarkPower)
	if err != nil {
		t.Fatal("did not create a merkle manager")
	}
	if MarkPower != MM1.MarkPower ||
		MarkFreq != MM1.MarkFreq ||
		MarkMask != MM1.MarkMask {
		t.Fatal("Marks were not correctly computed")
	}

	// Fill the Merkle Tree with a few hashes
	hash := sha256.Sum256([]byte("start"))
	for i := 0; i < testLen; i++ {
		require.NoError(t, MM1.AddHash(hash[:], false))
		hash = sha256.Sum256(hash[:])
	}

	if MM1.GetElementCount() != testLen {
		t.Fatal("added elements in merkle tree don't match the number we added")
	}

	// Sort the Indexing
	for i := int64(0); i < testLen; i++ {
		ms := MM1.GetState(i)
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

func GenerateTestData(prt bool) [10][]Hash {
	spaces := func(i int) {
		n := int(math.Pow(2, float64(i+1))) - 1
		for i := 0; i < n; i++ {
			print("          ")
		}
	}
	var rp common.RandHash
	var hashes [10][]Hash
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
			v := hashes[row][i].Combine(GetSha256(), hashes[row][i+1])
			hashes[row+1] = append(hashes[row+1], v)
			spaces(row)
			fmt.Printf("%3v ", v[:2])
		}
		row++
		println()
	}
	ms := new(MerkleState)
	ms.InitSha256()
	for _, v := range hashes[0] {
		ms.AddToMerkleTree(v)
		mdr := ms.GetMDRoot()
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
			v := hashes[row][i].Combine(GetSha256(), hashes[row][i+1])
			hashes[row+1][(i+1)/2] = v
			spaces(row)
			fmt.Printf("%x  ", v[:4])
		}
		row++
		println()
	}
	ms = new(MerkleState)
	ms.InitSha256()
	for _, v := range hashes[0] {
		ms.AddToMerkleTree(v)
		mdr := ms.GetMDRoot()
		fmt.Printf("%x  ", mdr[:4])
	}
	println()

	return hashes
}

func TestMerkleManager_GetIntermediate(t *testing.T) {
	store := database.OpenInMemory(nil)
	storeTx := store.Begin(true)
	m, _ := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 4)
	m.MS.InitSha256()

	hashes := GenerateTestData(true)

	var r common.RandHash

	for col := int64(0); col < 20; col++ {
		require.NoError(t, m.AddHash(r.NextList(), false))
		m.MS.PadPending()
		if col&1 == 1 {
			s, _ := m.GetAnyState(col - 1)
			s.PadPending()
			s.InitSha256()
			for row := int64(1); s.Pending[row-1] != nil; row++ {
				left, right, err := m.GetIntermediate(col, row)
				require.Nil(t, err, err)
				factor := int64(math.Pow(2, float64(row)))
				fmt.Printf("Row %d Col %d Left %x + Right %x == %x == Result %x\n",
					row, col, left[:4], right[:4],
					Hash(left).Combine(Sha256, right)[:4],
					hashes[row][col/factor][:4])
				require.True(t, bytes.Equal(Hash(left).Combine(Sha256, right), hashes[row][col/factor]), "should be equal")
			}
		}
	}

}

func TestMerkleManager_AddHash_Unique(t *testing.T) {
	var r common.RandHash
	hash := r.NextList()

	t.Run("true", func(t *testing.T) {
		store := database.OpenInMemory(nil)
		storeTx := store.Begin(true)
		m, _ := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 4)
		m.MS.InitSha256()

		require.NoError(t, m.AddHash(hash, true))
		require.NoError(t, m.AddHash(hash, true))
		require.Equal(t, int64(1), m.MS.Count)
	})

	t.Run("false", func(t *testing.T) {
		store := database.OpenInMemory(nil)
		storeTx := store.Begin(true)
		m, _ := NewMerkleManager(database.MerkleDbManager{Batch: storeTx}, 4)
		m.MS.InitSha256()

		require.NoError(t, m.AddHash(hash, false))
		require.NoError(t, m.AddHash(hash, false))
		require.Equal(t, int64(2), m.MS.Count)
	})
}

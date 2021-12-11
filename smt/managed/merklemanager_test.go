package managed

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"testing"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
	"github.com/stretchr/testify/require"
)

func TestMerkleManager_GetChainState(t *testing.T) {
	const testnum = 100
	var randHash RandHash
	dbm, e1 := database.NewDBManager("memory", "", nil)
	require.NoError(t, e1, "should be able to open a database")
	m, e2 := NewMerkleManager(dbm, 8)
	require.NoError(t, e2, "should be able to open a database")
	err := m.SetKey("try")
	require.NoError(t, err, "should be able to set base key")
	err = m.WriteChainHead(m.key...)
	require.NoError(t, err, "should be able to write to the chain head")
	head, err := m.ReadChainHead(m.key...)
	require.NoError(t, err, "should be able to read the chain head")
	require.True(t, head.Equal(m.MS), "chainstate should be loadable")

	var States []*MerkleState
	for i := 0; i < testnum; i++ {
		m.AddHash(randHash.Next())
		m.Manager.EndBatch()
		mState, err := m.MS.Marshal()
		require.NoError(t, err, "must be able to marshal a MerkleState")
		ms := new(MerkleState)
		err = ms.UnMarshal(mState)
		require.NoError(t, err, "must be able to unmarshal a MerkleState")
		require.True(t, ms.Equal(m.MS), " should get the same state back")
		cState, e2 := m.GetChainState(m.key...)
		require.NoErrorf(t, e2, "chain should always have a chain state %d", i)
		States = append(States, m.MS.Copy())
		require.Truef(t, cState.Equal(m.MS), "should be the last state of the chain written (%d)", i)
	}
}

func TestMerkleManager_GetAnyState(t *testing.T) {
	const testnum = 100
	var randHash RandHash
	dbm, e1 := database.NewDBManager("memory", "", nil)
	require.NoError(t, e1, "should be able to open a database")
	m, e2 := NewMerkleManager(dbm, 8)
	require.NoError(t, e2, "should be able to open a database")
	var States []*MerkleState
	for i := 0; i < testnum; i++ {
		m.AddHash(randHash.Next())
		States = append(States, m.MS.Copy())
	}
	for i := int64(0); i < testnum; i++ {
		state, err := m.GetAnyState(i)
		if err != nil {
			state, err = m.GetAnyState(i)
		}
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

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", "", nil); err != nil {
		t.Fatal(err)
	}

	Chain := sha256.Sum256([]byte("RedWagon/ACME_tokens"))
	BlkIdx := Chain
	BlkIdx[30] += 2

	MM1, err := NewMerkleManager(dbManager, 8)
	if err != nil {
		t.Fatal("didn't create a Merkle Manager")
	}

	if err := MM1.SetKey(Chain[:]); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < testlen; i++ {
		data := []byte(fmt.Sprintf("data %d", i))
		dataHash := sha256.Sum256(data)
		MM1.AddHash(dataHash[:])
		dataI, e := MM1.Manager.Key(Chain, "ElementIndex", dataHash).Get()
		if e != nil {
			t.Fatalf("error")
		}
		di, _ := common.BytesInt64(dataI)
		if di != int64(i) {
			t.Fatalf("didn't get the right index. got %d expected %d", di, i)
		}
		d, e2 := MM1.Manager.Key(Chain, "Element", i).Get()
		if e2 != nil || !bytes.Equal(d, dataHash[:]) {
			t.Fatalf("didn't get the data back. got %d expected %d", d, data)
		}
	}

}

func TestMerkleManager(t *testing.T) {

	const testLen = 1024

	dbManager := new(database.Manager)
	if err := dbManager.Init("memory", "", nil); err != nil {
		t.Fatal(err)
	}

	MarkPower := int64(2)
	MarkFreq := int64(math.Pow(2, float64(MarkPower)))
	MarkMask := MarkFreq - 1

	// Set up a MM1 that uses a MarkPower of 2
	MM1, err := NewMerkleManager(dbManager, MarkPower)
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
		MM1.AddHash(hash[:])
		hash = sha256.Sum256(hash[:])
	}

	if MM1.GetElementCount() != testLen {
		t.Fatal("added elements in merkle tree don't match the number we added")
	}

	dbManager.EndBatch()

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

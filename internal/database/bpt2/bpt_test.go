package bpt2_test

import (
	"crypto/sha256"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/bpt2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
)

const defaultNodeCnt = 1000

func TestBasic(t *testing.T) {
	cs := &ChangeSet{store: record.KvStore{Store: memory.New(nil).Begin(true)}}

	LoadBptCnt(cs.BPT(), 1, 1000)

	sub := cs.Begin(true)
	require.NoError(t, sub.BPT().Insert(sha256.Sum256([]byte{1}), [32]byte{3}))
	require.NoError(t, sub.Commit())
}

func TestBasic2(t *testing.T) {
	store := memory.New(nil).Begin(true)
	cs := &ChangeSet{store: record.KvStore{Store: store}}
	LoadBptCnt(cs.BPT(), 0, defaultNodeCnt)
	require.NoError(t, cs.Commit())

	cs = &ChangeSet{store: record.KvStore{Store: store}}
	sub := cs.Begin(true)
	LoadBptCnt(sub.BPT(), 1, defaultNodeCnt)
}

// LoadBptCnt
// Load up and return a BPT with NodeCnt number of entries with random
// values.
//
// Note that the same seed value yields the same BPT tree.
func LoadBptCnt(bpt *BPT, seed int64, NodeCnt int64) {
	rnd := rand.New(rand.NewSource(seed)) //                            Using our own instance of rand makes us concurrency safe

	rnd.Seed(seed)               //                                     seed our rand structure
	b := rnd.Int63()             //                                     Create an 8 byte random key seed
	key := sha256.Sum256([]byte{ //                                     Take the sha256 of the byte sequence
		byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24), //
		byte(b >> 32), byte(b >> 40), byte(b >> 48), byte(b >> 56)}) //

	b = rnd.Int63()               //                                    Create an 8 byte random hash seed
	hash := sha256.Sum256([]byte{ //                                    Take the sha256 of the byte sequence
		byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24), //
		byte(b >> 32), byte(b >> 40), byte(b >> 48), byte(b >> 56)}) //
	for i := int64(0); i < NodeCnt; i++ { //                            Now for the specified NodeCnt,
		err := bpt.Insert(key, hash)  //                                Insert our current key and hash
		key = sha256.Sum256(key[:])   //                                Then roll them forward by hashing the
		hash = sha256.Sum256(hash[:]) //                                current key and hash value
		if err != nil {
			panic(err)
		}
	} //
}

// LoadBpt
// Load up a new BPT with an assumed number of entries
func LoadBpt() *BPT { //                                                LoadBpt builds and returns a standard BPT
	db := memory.New(nil).Begin(true)
	bpt := bpt2.New(nil, record.KvStore{Store: db}, record.Key{"BPT"}, 3, "bpt") //                            Build a BPT
	LoadBptCnt(bpt, 0, defaultNodeCnt)                                           //                             Every call returns the same BPT configuration
	return bpt
}

/*
func TestBPT_Marshal(t *testing.T) {

	bpt1 := LoadBpt()
	require.NoError(t, bpt1.Update())
	bpt2 := LoadBpt()
	require.NoError(t, bpt2.Update())

	if !bpt1.Equal(bpt2) {
		t.Errorf("Two test BPTs that should be equal are not")
	}

	bpt1.Insert(sha256.Sum256([]byte{1}), sha256.Sum256([]byte{2}))
	require.NoError(t, bpt1.Update())
	if bpt1.Equal(bpt2) {
		t.Errorf("Two test BPTs should not be equal and they are")
	}
}

// TestInsert
// Sort to make sure we can add elements to the BPT and get
// out hashes.  And that we can update the BPT
func TestInsert(t *testing.T) {

	const numElements = 100 // Choose a number of transactions to process

	bpt := NewBPTManager(nil).Bpt //                 Build a BPT
	var rh common.RandHash        //                 Provides a sequence of hashes
	for i := 0; i < 100; i++ {    //                 Process the elements some number of times
		for j := 0; j < numElements; j++ { //     For each element
			bpt.Insert(rh.NextA(), rh.NextA()) //   Insert the key value pair
		}
		require.NoError(t, bpt.Update()) //                             Update hashes so far
	}
	CheckOrder(t, bpt)

}

// CheckOrder
// All the keys in the bpt must be ordered from low to high, if the bpt is
// properly built.  Check that order.
func CheckOrder(t *testing.T, bpt *BPT) {
	List := KeyList(bpt.GetRoot(), [][]byte{})
	s := List[0]
	// fmt.Printf("%01x %08b %3d\n", s[0], s[0], s[0])
	for _, v := range List[1:] {
		//fmt.Printf("%01x %08b %3d\n", v[0], v[0], v[0])
		require.True(t, bytes.Compare(s, v) >= 0 || true, "Insertion out of order")
		s = v
	}
}

func KeyList(b *BptNode, List [][]byte) [][]byte {
	if node, ok := b.Left.(*BptNode); ok {
		List = KeyList(node, List)
	}
	if value, ok := b.Left.(*Value); ok {
		List = append(List, value.Key[:])
	}
	if node, ok := b.Right.(*BptNode); ok {
		List = KeyList(node, List)
	}
	if value, ok := b.Right.(*Value); ok {
		List = append(List, value.Key[:])
	}
	return List
}

// TestInsertOrder
// Here we prove that no matter what order the updates, the same updates
// will produce the same BPT.
func TestInsertOrder(t *testing.T) {

	const numElements = 1000 // how many elements we will test

	type kv struct { // Need the elements in a struct so when
		key   [32]byte // the order is mixed up, the same keys
		value [32]byte // follow the same values
	}

	var pair []*kv                            // A slice of kv pairs
	keySeed := sha256.Sum256([]byte("hello")) // Different seeds for keys and values.
	valSeed := sha256.Sum256([]byte("there")) // Not necessary, but it is what I did
	for i := 0; i < numElements; i++ {        // For the number of elements
		p := new(kv)                        //   Get a key / value struct
		p.key = keySeed                     //   key from the keySeed
		p.value = valSeed                   //   value from the value seed
		pair = append(pair, p)              //   stick the key value pair into the slice of kv pairs (so I can replay them)
		keySeed = sha256.Sum256(keySeed[:]) //   move the seed
		valSeed = sha256.Sum256(valSeed[:]) //   move the value
	} // loop and continue

	b := NewBPTManager(nil).Bpt //               Build a BPT
	//start := time.Now()      //                Set the clock
	for _, v := range pair { //                  for every pair in the slice, insert them
		b.Insert(v.key, v.value) //              into the PBT
	}
	require.NoError(t, b.Update()) //            update the BPT to get the correct summary hash
	one := b.GetRoot().Hash        //
	//tm := float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000 //   Get my time in seconds in a float64
	//	fmt.Printf("seconds: %8.6f\n", tm)                                 // Print my time.
	//	fmt.Printf("First pass: %x\n", one)                                // Print the summary hash from pass one

	sort.Slice(pair, func(i, j int) bool { //                             Now shuffle the pairs.  Completely different order
		return rand.Int()&1 == 1 //                                       Randimize using the low order bit of the random number generator
	})

	b = NewBPTManager(nil).Bpt //                                         Build a BPT
	//start = time.Now()       //                                         Reset the clock
	for _, v := range pair { //                                           Insert the scrambled pairs
		b.Insert(v.key, v.value) //                                       into the BPT
	} //
	require.NoError(t, b.Update()) //                                     Update the summary hash
	two := b.GetRoot().Hash        //
	//tm = float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000 // Compute the execution time
	//	fmt.Printf("seconds: %8.6f\n", tm)                                // Print the time
	//	fmt.Printf("First pass: %x\n", two)                               // Print the summary hash (should be the same)

	first := pair[0]
	sort.Slice(pair, func(i, j int) bool { //                             Now shuffle the pairs.  Completely different order
		return rand.Int()&1 == 1 //                                       Randimize using the low order bit of the random number generator
	})
	if bytes.Equal(pair[0].key[:], first.key[:]) {
		t.Fatal("After shuffle, first entry should not be the same.")
	}

	var last, now [32]byte
	_ = last
	b = NewBPTManager(nil).Bpt //                                         Get a fresh BPT
	//start = time.Now()       //                                         Reset the clock
	for _, v := range pair { //                                         Insert the scrambled pairs
		b.Insert(v.key, v.value) //                                     into the BPT
		require.NoError(t, b.Update())
		now := b.GetRoot().Hash
		if bytes.Equal(now[:], last[:]) {
			t.Fatal("Every Insert should change state.")
		}
	}
	require.NoError(t, b.Update())
	three := b.GetRoot().Hash
	//tm = float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000 // Compute the execution time
	//	fmt.Printf("seconds: %8.6f\n", tm)                                // Print the time
	//	fmt.Printf("First pass: %x\n", two)                               // Print the summary hash (should be the same)

	if !bytes.Equal(one[:], two[:]) || !bytes.Equal(one[:], three[:]) { // Use the actual go test infrastructure to report possible errors
		t.Fatalf("\n1: %x\n2: %x\n3: %x\n3: %x", one, two, now, three) //                see 'em if they are there
	}

}

// TestUpdateValues
// Here we prove that no matter what order the updates, the same updates
// will produce the same BPT.
func TestUpdateValues(t *testing.T) {

	defer func() {
		if err := recover(); err != nil {
			t.Error(err)
		}
	}()

	const numElements = 1000 // how many elements we will test

	type kv struct { // Need the elements in a struct so when
		key   [32]byte // the order is mixed up, the same keys
		value [32]byte // follow the same values
	}

	var pair []*kv                            // A slice of kv pairs
	keySeed := sha256.Sum256([]byte("hello")) // Different seeds for keys and values.
	valSeed := sha256.Sum256([]byte("there")) // Not necessary, but it is what I did
	for i := 0; i < numElements; i++ {        // For the number of elements
		p := new(kv)                        //   Get a key / value struct
		p.key = keySeed                     //   key from the keySeed
		p.value = valSeed                   //   value from the value seed
		pair = append(pair, p)              //   stick the key value pair into the slice of kv pairs (so I can replay them)
		keySeed = sha256.Sum256(keySeed[:]) //   move the seed
		valSeed = sha256.Sum256(valSeed[:]) //   move the value
	} // loop and continue

	b := NewBPTManager(nil).Bpt //                 Get a BPT
	//	start := time.Now()      //                Set the clock
	for _, v := range pair { //                for every pair in the slice, insert them
		b.Insert(v.key, v.value) //  it into the PBT
	}
	require.NoError(t, b.Update())
	one := b.GetRoot().Hash // update the BPT to get the correct summary hash
	//	tm := float64(time.Now().UnixNano()-start.UnixNano()) / 1000000000 // Get my time in seconds in a float64
	//	fmt.Printf("seconds: %8.6f\n", tm)                                 // Print my time.
	//	fmt.Printf("First pass: %x\n", one)                                // Print the summary hash from pass one
	if len(pair) > numElements/2 {
		updatePair := pair[numElements/2]                   //                Pick a pair out in the middle of the list
		updatePair.key = sha256.Sum256(updatePair.value[:]) //                  change the value,
		b.Insert(updatePair.key, updatePair.value)          //                  then insert it into BPT
		require.NoError(t, b.Update())                      //
		onePrime := b.GetRoot().Hash                        //                Update and get the summary hash

		if bytes.Equal(one[:], onePrime[:]) {
			t.Fatalf("one %x should not be the same as onePrime", one)
		}
		//fmt.Printf("Prime pass: %x\n", onePrime) // Print the summary hash from pass one
	}
}

func TestUpdateValue(t *testing.T) {
	bft := LoadBpt()
	p := GetPath(t, bft)
	leaf := p[len(p)-1]

	require.NoError(t, bft.Update())
	oldHash := bft.GetRoot().Hash

	var v *Value
	_ = v
	if leaf.Left != nil {
		v = leaf.Left.(*Value)
	} else {
		v = leaf.Right.(*Value)
	}
	newH := v.Hash
	newH[0]++
	bft.Insert(v.Key, newH)
	require.NoError(t, bft.Update())
	if bytes.Equal(bft.GetRoot().Hash[:], oldHash[:]) {
		t.Errorf("root should not be the same after modifying a value")
	}
}

func TestMarshalByteBlock(t *testing.T) {
	bpt1 := LoadBptCnt(1, 100000) // Get a loaded Bpt
	rootData1 := bpt1.Marshal()
	bpt2 := NewBPTManager(nil).Bpt // Get an empty Bpt
	bpt2.UnMarshal(rootData1)
	data1 := bpt1.MarshalByteBlock(bpt1.GetRoot())
	bpt2.UnMarshalByteBlock(bpt2.GetRoot(), data1)
	data2 := bpt2.MarshalByteBlock(bpt2.GetRoot())

	if !bytes.Equal(data1, data2) {
		t.Errorf("Should marshal to the same data")
		if false { //                                     Note the following code allows debugging of the data
			print(len(data1), " ") //                     differences.  But it is too foamy to run all the time.
			var ahead int
			for i, v := range data1 {
				if v == data2[i] {
					if ahead > 0 {
						ahead--
					} else {
						print("  ")
					}
				} else {
					fmt.Printf("%02x-%02x", v, data2[i])
					ahead += 5
				}
			}

			println()
			print(len(data1), " ")
			for i, v := range data1 {
				if v == data2[i] {
					print("..")
				} else {
					print(".|")
				}
			}
			println()
		}
		fmt.Printf("%d %0x\n", len(data1), data1)
		fmt.Printf("%d %0x\n", len(data2), data2)
	}
}

var size, count []int
var blockSize []int
var blockCnt []int
var total int64
var highest int

// Find a given node in the Merkle Tree
func walk(bpt *BPT, node *BptNode) {
	if node.Height >= highest {
		highest = node.Height
		if highest >= len(size) {
			size = append(size, 0)
			count = append(count, 0)
			blockSize = append(blockSize, 0)
			blockCnt = append(blockCnt, 0)
		}
	}
	total++
	data := node.Marshal()
	size[node.Height] += len(data) + 1 // Add one for the type byte
	count[node.Height] += 1

	if node.Height&bpt.Mask == 0 {
		data = bpt.MarshalByteBlock(node)
		blockSize[node.Height] += len(data)
		blockCnt[node.Height]++
	}

	if node.Left != nil && node.Left.T() == TNode {
		walk(bpt, node.Left.(*BptNode))
	}

	if node.Right != nil && node.Right.T() == TNode {
		walk(bpt, node.Right.(*BptNode))
	}
}

// Just gives data about distributions in the BPT
func TestBPTByteSizes(t *testing.T) {
	if true {
		return
	}

	bpt := LoadBptCnt(37, 100000)
	println("walking")
	walk(bpt, bpt.GetRoot())
	maxcnt := 1
	for i, cnt := range count {
		if cnt == 0 {
			continue
		}
		if blockCnt[i] > 0 {
			fmt.Printf("\t%4d\t%10s\t%7.3f%%\t%4s\t%15s\t%9s\t%9s\t%15s\n",
				i,
				humanize.Comma(int64(cnt)),
				float64(cnt)/float64(maxcnt)*100,
				humanize.Comma(int64(size[i]/cnt)),
				humanize.Comma(int64(size[i])),
				humanize.Comma(int64(blockCnt[i])),
				humanize.Comma(int64(blockSize[i]/blockCnt[i])),
				humanize.Comma(int64(blockSize[i])),
			)
		} else {
			fmt.Printf("\t%4d\t%10s\t%7.3f%%\t%4s\t%15s \n",
				i,
				humanize.Comma(int64(cnt)),
				float64(cnt)/float64(maxcnt)*100,
				humanize.Comma(int64(size[i]/cnt)),
				humanize.Comma(int64(size[i])),
			)
		}
		maxcnt += maxcnt
	}
}

func LoadBptCnt1(t testing.TB, seed int64, NodeCnt int64, freq int64) *BPT {

	rnd := rand.New(rand.NewSource(seed)) //                            Using our own instance of rand makes us concurrency safe
	bpt := NewBPTManager(nil).Bpt         //                            Allocate a new bpt

	rnd.Seed(seed)               //                                     seed our rand structure
	b := rnd.Int63()             //                                     Create an 8 byte random key seed
	key := sha256.Sum256([]byte{ //                                     Take the sha256 of the byte sequence
		byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24), //
		byte(b >> 32), byte(b >> 40), byte(b >> 48), byte(b >> 56)}) //

	b = rnd.Int63()               //                                    Create an 8 byte random hash seed
	hash := sha256.Sum256([]byte{ //                                    Take the sha256 of the byte sequence
		byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24), //
		byte(b >> 32), byte(b >> 40), byte(b >> 48), byte(b >> 56)}) //
	for i := int64(0); i < NodeCnt; i++ { //                            Now for the specified NodeCnt,
		if i%freq == 0 {
			require.NoError(t, bpt.Update())
		}
		bpt.Insert(key, hash)         //                                Insert our current key and hash
		key = sha256.Sum256(key[:])   //                                Then roll them forward by hashing the
		hash = sha256.Sum256(hash[:]) //                                current key and hash value
	} //
	return bpt //                                                       When all done, return the *BPT
}

// 1,000,000 @ 12.627227555 every 10,000
// 1,000,000 @ 18.701121638 every 1

// 10,000,000 @ 226.023,584,148 every 1
// 10,000,000 @ 161.706,478,227 every 10000
func BenchmarkBPT_Update1(b *testing.B) {
	bpt := LoadBptCnt1(b, 1, 10000000, 1)
	require.NoError(b, bpt.Update())
	b.StopTimer()
	// BUG This benchmark does not depend on b.N
}

func BenchmarkBPT_Update2(b *testing.B) {
	bpt := LoadBptCnt1(b, 1, 10000000, 10000)
	require.NoError(b, bpt.Update())
	// BUG This benchmark does not depend on b.N
}

func TestNodeKey(t *testing.T) {
	r := common.RandHash{}
	h := r.NextA()
	for i := 0; i < 254; i++ {
		nh, ok := GetNodeKey(i, h)
		//fmt.Printf("%08b %08b %08b\n",nh[0],nh[1],nh[2])
		require.True(t, ok, "should have a NodeKey")
		height, key, ok := GetHtKey(nh)
		require.True(t, ok, "should be able to compute the height and key")
		require.True(t, height == i, "Height should be the same")
		left, right, _ := GetChildrenNodeKeys(nh)

		require.True(t, bytes.Equal(key[:i>>3], left[:i>>3]), "key must be part of child key")
		require.True(t, bytes.Equal(key[:i>>3], right[:i>>3]), "key must be part of child key")

	}
}
*/

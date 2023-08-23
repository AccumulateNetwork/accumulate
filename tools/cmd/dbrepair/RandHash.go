// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"
)

// RandHash
// Creates a deterministic stream of hashes.  We need this all over our tests
// and though this is late to the party, it is very useful in testing in
// SMT
type RandHash struct {
	seed [32]byte
	List [][]byte
}

// GetRandInt64
// Really returns a 63 bit number, as the return value is always positive.
func (n *RandHash) GetRandInt64() int64 {
	next := n.Next()
	return ((((((((int64(next[0])&0x7F)<<8)+ // And with 0x7F to clear high bit
		int64(next[1]))<<8+
		int64(next[2]))<<8+
		int64(next[3]))<<8+
		int64(next[4]))<<8+
		int64(next[5]))<<8+
		int64(next[6]))<<8 +
		int64(next[7])
}

// GetRandBuff
// return a buffer of random data of the given size.
// Okay, we could do this much more efficiently, but meh!
func (n *RandHash) GetRandBuff(size int) (buff []byte) {
	for {
		next := n.Next()          //      Random values come from hashing
		if size-len(buff) >= 32 { //      do all the 32 byte lengths
			buff = append(buff, next...)
			continue
		}
		for _, b := range next { //       once less than 32 bytes, do it a byte at time
			if len(buff) == size {
				return buff
			}
			buff = append(buff, b)
		}

	}
}

// GetIntN
// Return an int between 0 and N
func (n *RandHash) GetIntN(N int) int {
	next := n.Next()
	r := int(binary.BigEndian.Uint64(next) % uint64(N))
	return r
}

// GetAElement
// Get an Array of an element out of the list of hashes
func (n *RandHash) GetAElement(i int) (A [32]byte) {
	copy(A[:], n.List[i])
	return A
}

// SetSeed
// If the tester wishes a different sequence of hashes, a seed can be
// specified
func (n *RandHash) SetSeed(seed []byte) {
	n.seed = sha256.Sum256(seed)
}

// Next
// Returns the next hash in a deterministic sequence of hashes
func (n *RandHash) Next() []byte {
	n.seed = sha256.Sum256(n.seed[:])
	return append([]byte{}, n.seed[:]...)
}

// NextA
// Returns the next hash array in a deterministic sequence of hashes
func (n *RandHash) NextA() (A [32]byte) {
	n.seed = sha256.Sum256(n.seed[:])
	A = n.seed
	return A
}

// NextList
// Just like Next, but each hash is logged in RandHash.List
func (n *RandHash) NextList() []byte {
	h := n.Next()
	n.List = append(n.List, h)
	return h
}

// NextAList
// Just like NextA, but each ash is logged in RandHash.ListA
func (n *RandHash) NextAList() (A [32]byte) {
	h := n.NextList()
	copy(A[:], h)
	return A
}

// If you just use RandHash with the default seed of nil, you can count
// on the following sequence of hashes, and the following values within a
// Merkle Tree.
// Because GoLand is terrible about displaces of byte arrays, the following
// provides the first two bytes of a byte array, as well as the hexadecimal
// four bytes for every hash.
/*

    0         1          2        3          4         5         6         7         8         9        10        11        12        13        14	15        16        17        18	  19
[102 104] [ 43  50] [ 18 119] [254  21] [ 55 109] [ 67 145] [ 93  26] [106 155] [ 78 110] [241  53] [123  31] [220 178] [152  96] [146 219] [ 33 103] [ 45  62] [ 67  25] [196  33] [167 253] [152  33]
          [ 27 215]           [ 92 185]           [ 73 247]           [ 33 145]           [241 160]           [125  23]           [227 240]           [237 122]           [241 174]           [ 97 170]
                              [133  30]                               [ 40  93]                               [123  91]                               [124  37]                               [ 13  79]
                                                                      [217 167]                                                                       [ 87  74]
                                                                                                                                                      [157 195]
[102 104] [ 27 215] [173 207] [133  30] [ 40  86] [ 81 255] [ 97 195] [217 167] [ 78   3] [174 110] [230   6] [212  44] [111 112] [166  21] [203 252] [157 195] [ 42 205] [ 34 209] [ 98 144] [ 21  54]

66687aad  2b32db6c  12771355  fe15c0d3  376da11f  4391a5c7  5d1adcb5  6a9b711c  4e6e6ace  f13587bc  7b1f7c3b  dcb20558  986044cc  92db992e  2167a4ac  2d3ed015  43198db7  c4217d57  a7fd40e1  98211882
          1bd70ade            5cb91123            49f7ba44            21910837            f1a05114            7d17df99            e3f05f75            ed7afce4            f1ae63d1            61aa3b79
                              851e6d6c                                285d6376                                7b5b27dd                                7c257491                                0d4fa70e
                                                                      d9a71d2c                                                                        574ae276
                                                                                                                                                      9dc338e0
66687aad  1bd70ade  adcff944  851e6d6c  285693b4  51ff81ba  61c3b3f6  d9a71d2c  4e031d86  ae6e0bc4  e60693cc  d42c6e20  6f70b0c7  a6154564  cbfc3b80  9dc338e0  2acdab55  22d133dd  62902a20  15362c17
*/

// TestReceipt_PrintChart
// produces the chart below (without the periods)t *testing.T) {

func TestReceipt_PrintChart(t *testing.T) {
	for i := uint(0); i < 20; i++ {
		fmt.Printf("%3d ", i)
	}
	println()
	for level := uint(0); level < 5; level++ {
		d := " "
		for i := uint(1); i < 21; i++ {
			mask := ^(^uint(0) << level)
			if d == "R" || d == "." {
				d = "."
			} else {
				d = " "
			}
			if i&mask == 0 {
				d = "L"
				if (i>>level)&1 == 1 {
					d = "R"
				}
			}
			fmt.Printf("%3s ", d)
		}
		println()
	}
	println()

}

/*

  0   1   2   3   4   5   6   7   8   9  10  11  12  13  14  15  16  17  18  19
  R   L   R   L   R   L   R   L   R   L   R   L   R   L   R   L   R   L   R   L
      R   .   L       R   .   L       R   .   L       R   .   L       R   .   L
              R   .   .   .   L               R   .   .   .   L               R
                              R   .   .   .   .   .   .   .   L
                                                              R   .   .   .   .

This chart maps the nodes in a Merkle Tree where nodes are labeled "R" to denote
that they are combined with the "L" to the Right of that node, even if separated
by '.', A Node labeled "L" is combined with the node ro its right and labeled
"R". So given:
         R   .    .   L
                      X
Node X (be it an R Node or an L Node) is produced by combining R with L.

In MerkleState.Pending, each list is terminated by an R entry in the column.
Note that blanks reflect nils (or entries past the end of Pending).  Dots
also represent the value of R to the left.

R   .   .   L

The above dots represent R in the pending list in the columns with the dots
between the R and the L nodes.


Note that in the code, X is specified by an element index and a height.  What
is returned uses Left and Right from the perspective of X.  So R is the node
on the left, and L is the node on the right, looking at these nodes from X.

func (*MerkleManager) GetIntermediate(element, height int64) (Left,Right Hash, err error)
*/

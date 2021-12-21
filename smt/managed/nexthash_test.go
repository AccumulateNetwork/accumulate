package managed

import "crypto/sha256"

// RandHash
// Creates a deterministic stream of hashes.  We need this all over our tests
// and though this is late to the party, it is very useful in testing in
// SMT
type RandHash struct {
	seed [32]byte
	List [][]byte
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

// Next
// Just like Next, but each hash is logged in RandHash.List
func (n *RandHash) NextList() []byte {
	h := n.Next()
	n.List = append(n.List, h)
	return h
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

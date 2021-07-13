package factom

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBlock(t *testing.T) {
	for _, test := range DBlockTests {
		t.Run(test.Name, func(t *testing.T) {
			testDBlock(t, test)
		})
	}

	t.Run("random", func(t *testing.T) {
		buf := make([]byte, DBlockMinTotalSize*10)
		for i := 0; i < 5000; i++ {
			d := buf[:rand.Intn(DBlockMinTotalSize*9)+DBlockMinBodySize-4]
			rand.Read(d)

			d[0] = 0

			var db DBlock
			require.Error(t, db.UnmarshalBinary(d), "UnmarshalBinary")
		}
	})
}

func testDBlock(t *testing.T, test DBlockTest) {
	require := require.New(t)
	var db DBlock
	err := db.UnmarshalBinary(test.Data)
	if test.Err != "" {
		require.EqualErrorf(err, test.Err, "DBlock.UnmarshalBinary")
		return
	}
	require.NoError(err)

	assert := assert.New(t)
	assert.Equal(test.Exp.KeyMR[:], db.KeyMR[:], "KeyMR")
	assert.Equal(test.Exp.PrevKeyMR[:], db.PrevKeyMR[:], "PrevKeyMR")
	assert.Equal(test.Exp.BodyMR[:], db.BodyMR[:], "BodyMR")
	assert.Equal(test.Exp.FullHash[:], db.FullHash[:], "FullHash")
	assert.Equal(test.Exp.PrevFullHash[:], db.PrevFullHash[:], "FullHash")
	assert.Equal(test.Exp.Timestamp, db.Timestamp, "Timestamp")
	assert.Equal(test.Exp.Timestamp, db.FBlock.Timestamp, "FBlock.Timestamp")
	assert.Equal(test.Exp.Height, db.Height, "Height")
	assert.Equal(test.Exp.Height, db.FBlock.Height, "FBlock.Height")
	assert.Equal(test.Exp.FBlock.KeyMR[:], db.FBlock.KeyMR[:], "FBlock.KeyMR")

	for i, expEB := range test.Exp.EBlocks {
		eb := db.EBlocks[i]
		assert.Equalf(expEB.ChainID[:], eb.ChainID[:], "EBlock[%v].ChainID", i)
		assert.Equalf(expEB.KeyMR[:], eb.KeyMR[:], "EBlock[%v].KeyMR", i)
		assert.Equalf(test.Exp.Timestamp, eb.Timestamp, "EBlock[%v].Timestamp", i)
		assert.Equalf(test.Exp.Height, eb.Height, "EBlock[%v].Height", i)
	}

	data, err := db.MarshalBinary()
	require.NoError(err, "MarshalBinary (cached)")
	assert.Equal(Bytes(test.Data), Bytes(data), "MarshalBinary (cached)")

	db.ClearMarshalBinaryCache()
	data, err = db.MarshalBinary()
	require.NoError(err, "MarshalBinary")
	assert.Equal(Bytes(test.Data), Bytes(data), "MarshalBinary")
}

type DBlockTest struct {
	Name string
	Data Bytes
	Exp  DBlock
	Err  string
}

func b32(s string) *Bytes32 {
	b32 := NewBytes32(s)
	return &b32
}

var b = NewBytes

func name(name string) string {
	_, _, line, ok := runtime.Caller(1)
	if !ok {
		panic("couldn't get line number")
	}
	return fmt.Sprintf("%s line %v", name, line)
}

var DBlockTests = []DBlockTest{
	{
		Name: name("valid mainnet dbheight 1000"),
		Data: b("00fa92e5a2f2eaf170a2da9e4956a40231ed7255c6c6e5ada1ed746fc5ab3a0b79b8c700367a49467be900ba00daedd7d9cf2b1a07f839360e859e1f3d78c46701d3ad1507974595bf9b73dbec9ff5d5744cbf6410d66b837924208a0b8b84e54fc4aad660016ea716000003e800000004000000000000000000000000000000000000000000000000000000000000000a3d92dc70f4cfd4fe464e18962057d71924679cc866fe37f4b8023d292d9f34ce000000000000000000000000000000000000000000000000000000000000000c0526c1fdb9e0813e297a331891815ed893cb5a9cff15529197f13932ed9f9547000000000000000000000000000000000000000000000000000000000000000f526aca5f63bfb59188bae1fc367411a123bcc1d5a3c23c710b66b46703542855df3ade9eec4b08d5379cc64270c30ea7315d8a8a1a69efe2b98a60ecdd69e604f08c42bc44c09ac26c349bef8ee80d2ffb018cfa3e769107b2413792fa9bd642"),
		Exp: DBlock{
			NetworkID:    MainnetID(),
			Height:       1000,
			Timestamp:    time.Unix(24028950*60, 0),
			KeyMR:        b32("cd45e38f53c090a03513f0c67afb93c774a064a5614a772cd079f31b3db4d011"),
			BodyMR:       b32("f2eaf170a2da9e4956a40231ed7255c6c6e5ada1ed746fc5ab3a0b79b8c70036"),
			FullHash:     b32("06e8d2d429fe728c4a90a3b6fbd910eb97e543c460c762a72d1563302bb401b1"),
			PrevKeyMR:    b32("7a49467be900ba00daedd7d9cf2b1a07f839360e859e1f3d78c46701d3ad1507"),
			PrevFullHash: b32("974595bf9b73dbec9ff5d5744cbf6410d66b837924208a0b8b84e54fc4aad660"),
			FBlock:       FBlock{KeyMR: b32("526aca5f63bfb59188bae1fc367411a123bcc1d5a3c23c710b66b46703542855")},
			EBlocks: []EBlock{
				{
					ChainID: b32("000000000000000000000000000000000000000000000000000000000000000a"),
					KeyMR:   b32("3d92dc70f4cfd4fe464e18962057d71924679cc866fe37f4b8023d292d9f34ce"),
				}, {
					ChainID: b32("000000000000000000000000000000000000000000000000000000000000000c"),
					KeyMR:   b32("0526c1fdb9e0813e297a331891815ed893cb5a9cff15529197f13932ed9f9547"),
				}, {
					ChainID: b32("df3ade9eec4b08d5379cc64270c30ea7315d8a8a1a69efe2b98a60ecdd69e604"),
					KeyMR:   b32("f08c42bc44c09ac26c349bef8ee80d2ffb018cfa3e769107b2413792fa9bd642"),
				},
			},
		},
	},
	{
		Name: name("valid mainnet dbheight 10000"),
		Data: b("00fa92e5a26b0b1f81709569b0786df580962b65a00bc9f5ca78276828daf44a00fdbac5594e623a42c8f79fb74e070c185a8b1ad840b315508089b7a3414b32d42bc3b4895c7e9df048518d48be91324dbd2860dc6f685c733515c30b5467d6005c4f97140170075a0000271000000005000000000000000000000000000000000000000000000000000000000000000a9ac34d410e50f7151d297a5068fa367a6c7e1ab18922fe128bb60ce3382f90de000000000000000000000000000000000000000000000000000000000000000cb2bae18160eeb695f8039b5466772732b19803e809d92978efe6e736247e12e0000000000000000000000000000000000000000000000000000000000000000f3b253ec8e25c25752fbc9ae5382bbf953051792b3805ed22f0f92734647aeefe23985c922e9cdd5ec09c7f52a7c715bc9e26295778ead5d54e30a0a6215783c85fa74871f36fb59b226ba06ca5d9a22bb0295fe1714635cfa7080dd35ed117944bf71c177e71504032ab84023d8afc16e302de970e6be110dac20adbf9a197467f0fc1f1b295c190f1f8b7d605f7b0800b42c1259a0fd10bcee302345d70134c"),
		Exp: DBlock{
			NetworkID:    MainnetID(),
			Height:       10000,
			Timestamp:    time.Unix(24119130*60, 0),
			KeyMR:        b32("3670a63eb8051b925213a4a350e8d37d87e43da8a577a609d7fd30629b73a3aa"),
			BodyMR:       b32("6b0b1f81709569b0786df580962b65a00bc9f5ca78276828daf44a00fdbac559"),
			FullHash:     b32("3dcd03c72ef44c5d1a4ea6c10882a0e88354236ea5a55325b146636d00da8e50"),
			PrevKeyMR:    b32("4e623a42c8f79fb74e070c185a8b1ad840b315508089b7a3414b32d42bc3b489"),
			PrevFullHash: b32("5c7e9df048518d48be91324dbd2860dc6f685c733515c30b5467d6005c4f9714"),
			FBlock:       FBlock{KeyMR: b32("3b253ec8e25c25752fbc9ae5382bbf953051792b3805ed22f0f92734647aeefe")},
			EBlocks: []EBlock{
				{
					ChainID: b32("000000000000000000000000000000000000000000000000000000000000000a"),
					KeyMR:   b32("9ac34d410e50f7151d297a5068fa367a6c7e1ab18922fe128bb60ce3382f90de"),
				}, {
					ChainID: b32("000000000000000000000000000000000000000000000000000000000000000c"),
					KeyMR:   b32("b2bae18160eeb695f8039b5466772732b19803e809d92978efe6e736247e12e0"),
				}, {
					ChainID: b32("23985c922e9cdd5ec09c7f52a7c715bc9e26295778ead5d54e30a0a6215783c8"),
					KeyMR:   b32("5fa74871f36fb59b226ba06ca5d9a22bb0295fe1714635cfa7080dd35ed11794"),
				}, {
					ChainID: b32("4bf71c177e71504032ab84023d8afc16e302de970e6be110dac20adbf9a19746"),
					KeyMR:   b32("7f0fc1f1b295c190f1f8b7d605f7b0800b42c1259a0fd10bcee302345d70134c"),
				},
			},
		},
	}, {
		Name: name("invalid out of order chain ids"),
		Data: b("00fa92e5a26b0b1f81709569b0786df580962b65a00bc9f5ca78276828daf44a00fdbac5594e623a42c8f79fb74e070c185a8b1ad840b315508089b7a3414b32d42bc3b4895c7e9df048518d48be91324dbd2860dc6f685c733515c30b5467d6005c4f97140170075a0000271000000005000000000000000000000000000000000000000000000000000000000000000cb2bae18160eeb695f8039b5466772732b19803e809d92978efe6e736247e12e0000000000000000000000000000000000000000000000000000000000000000f000000000000000000000000000000000000000000000000000000000000000a9ac34d410e50f7151d297a5068fa367a6c7e1ab18922fe128bb60ce3382f90de3b253ec8e25c25752fbc9ae5382bbf953051792b3805ed22f0f92734647aeefe23985c922e9cdd5ec09c7f52a7c715bc9e26295778ead5d54e30a0a6215783c85fa74871f36fb59b226ba06ca5d9a22bb0295fe1714635cfa7080dd35ed117944bf71c177e71504032ab84023d8afc16e302de970e6be110dac20adbf9a197467f0fc1f1b295c190f1f8b7d605f7b0800b42c1259a0fd10bcee302345d70134c"),
		Err:  "out of order or duplicate Chain ID",
	},
}

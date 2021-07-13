// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

// Code generated DO NOT EDIT

package factom

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	// Test id/sk key pairs with all zeros.
	// OBVIOUSLY NEVER USE THESE FOR ANYTHING!
	id1KeyStr = "id12K4tCXKcJJYxJmZ1UY9EuKPvtGVAjo32xySMKNUahbmRcsqFgW"
	id2KeyStr = "id22pNvsaMWf9qxWFrmfQpwFJiKQoWfKmBwVgQtdvqVZuqzGmrFNY"
	id3KeyStr = "id33pRgpm8ufXNGxtW7n5FgdGP6afXKjU4LfVmgfC8Yaq6LyYq2wA"
	id4KeyStr = "id42vYqBB63eoSz8DHozEwtCaLbEwvBTG9pWgD3D5CCaHWy1gCjF5"

	sk1KeyStr = "sk13iLKJfxNQg8vpSmjacEgEQAnXkn7rbjd5ewexc1Un5wVPa7KTk"
	sk2KeyStr = "sk22UaDys2Mzg2pUCsToo9aKgxubJFnZN5Bc2LXfV59VxMvXXKwXa"
	sk3KeyStr = "sk32Xyo9kmjtNqRUfRd3ZhU56NZd8M1nR61tdBaCLSQRdhUCk4yiM"
	sk4KeyStr = "sk43eMusQuvvChoGNn1VZZwbAH8BtKJSZNC7ZWoz1Vc4Y3greLA45"
)

type idKeyUnmarshalJSONTest struct {
	Name  string
	ID    interface{}
	ExpID interface{}
	Data  string
	Err   string
}

var idKeyUnmarshalJSONTests = []idKeyUnmarshalJSONTest{{
	Name: "valid/ID1",
	Data: fmt.Sprintf("%q", id1KeyStr),
	ID:   new(ID1Key),
	ExpID: func() *ID1Key {
		sk, _ := NewSK1Key(sk1KeyStr)
		id := sk.ID1Key()
		return &id
	}(),
}, {
	Name: "invalid type/ID1",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.ID1Key",
	ID:   new(ID1Key),
}, {
	Name: "invalid type/ID1",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.ID1Key",
	ID:   new(ID1Key),
}, {
	Name: "invalid type/ID1",
	Data: `["hello"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.ID1Key",
	ID:   new(ID1Key),
}, {
	Name: "valid/ID2",
	Data: fmt.Sprintf("%q", id2KeyStr),
	ID:   new(ID2Key),
	ExpID: func() *ID2Key {
		sk, _ := NewSK2Key(sk2KeyStr)
		id := sk.ID2Key()
		return &id
	}(),
}, {
	Name: "invalid type/ID2",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.ID2Key",
	ID:   new(ID2Key),
}, {
	Name: "invalid type/ID2",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.ID2Key",
	ID:   new(ID2Key),
}, {
	Name: "invalid type/ID2",
	Data: `["hello"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.ID2Key",
	ID:   new(ID2Key),
}, {
	Name: "valid/ID3",
	Data: fmt.Sprintf("%q", id3KeyStr),
	ID:   new(ID3Key),
	ExpID: func() *ID3Key {
		sk, _ := NewSK3Key(sk3KeyStr)
		id := sk.ID3Key()
		return &id
	}(),
}, {
	Name: "invalid type/ID3",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.ID3Key",
	ID:   new(ID3Key),
}, {
	Name: "invalid type/ID3",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.ID3Key",
	ID:   new(ID3Key),
}, {
	Name: "invalid type/ID3",
	Data: `["hello"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.ID3Key",
	ID:   new(ID3Key),
}, {
	Name: "valid/ID4",
	Data: fmt.Sprintf("%q", id4KeyStr),
	ID:   new(ID4Key),
	ExpID: func() *ID4Key {
		sk, _ := NewSK4Key(sk4KeyStr)
		id := sk.ID4Key()
		return &id
	}(),
}, {
	Name: "invalid type/ID4",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.ID4Key",
	ID:   new(ID4Key),
}, {
	Name: "invalid type/ID4",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.ID4Key",
	ID:   new(ID4Key),
}, {
	Name: "invalid type/ID4",
	Data: `["hello"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.ID4Key",
	ID:   new(ID4Key),
}, {

	Name: "valid/SK1",
	Data: fmt.Sprintf("%q", sk1KeyStr),
	ID:   new(SK1Key),
	ExpID: func() *SK1Key {
		key, _ := NewSK1Key(sk1KeyStr)
		return &key
	}(),
}, {
	Name: "invalid type/SK1",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.SK1Key",
	ID:   new(SK1Key),
}, {
	Name: "invalid type/SK1",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.SK1Key",
	ID:   new(SK1Key),
}, {
	Name: "invalid type/SK1",
	Data: `["hello"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.SK1Key",
	ID:   new(SK1Key),
}, {
	Name: "valid/SK2",
	Data: fmt.Sprintf("%q", sk2KeyStr),
	ID:   new(SK2Key),
	ExpID: func() *SK2Key {
		key, _ := NewSK2Key(sk2KeyStr)
		return &key
	}(),
}, {
	Name: "invalid type/SK2",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.SK2Key",
	ID:   new(SK2Key),
}, {
	Name: "invalid type/SK2",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.SK2Key",
	ID:   new(SK2Key),
}, {
	Name: "invalid type/SK2",
	Data: `["hello"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.SK2Key",
	ID:   new(SK2Key),
}, {
	Name: "valid/SK3",
	Data: fmt.Sprintf("%q", sk3KeyStr),
	ID:   new(SK3Key),
	ExpID: func() *SK3Key {
		key, _ := NewSK3Key(sk3KeyStr)
		return &key
	}(),
}, {
	Name: "invalid type/SK3",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.SK3Key",
	ID:   new(SK3Key),
}, {
	Name: "invalid type/SK3",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.SK3Key",
	ID:   new(SK3Key),
}, {
	Name: "invalid type/SK3",
	Data: `["hello"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.SK3Key",
	ID:   new(SK3Key),
}, {
	Name: "valid/SK4",
	Data: fmt.Sprintf("%q", sk4KeyStr),
	ID:   new(SK4Key),
	ExpID: func() *SK4Key {
		key, _ := NewSK4Key(sk4KeyStr)
		return &key
	}(),
}, {
	Name: "invalid type/SK4",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.SK4Key",
	ID:   new(SK4Key),
}, {
	Name: "invalid type/SK4",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.SK4Key",
	ID:   new(SK4Key),
}, {
	Name: "invalid type/SK4",
	Data: `["hello"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.SK4Key",
	ID:   new(SK4Key),
}, {

	Name: "invalid length",
	Data: fmt.Sprintf("%q", id1KeyStr[0:len(id1KeyStr)-1]),
	Err:  "invalid length",
}, {
	Name: "invalid length",
	Data: fmt.Sprintf("%q", id1KeyStr+"Q"),
	Err:  "invalid length",
}, {
	Name: "invalid prefix",
	Data: fmt.Sprintf("%q", func() string {
		key, _ := NewSK1Key(sk1KeyStr)
		return key.payload().StringWithPrefix([]byte{0x50, 0x50, 0x50})
	}()),
	Err: "invalid prefix",
}, {
	Name:  "invalid symbol/ID1",
	Data:  fmt.Sprintf("%q", id1KeyStr[0:len(id1KeyStr)-1]+"0"),
	Err:   "invalid format: version and/or checksum bytes missing",
	ID:    new(ID1Key),
	ExpID: new(ID1Key),
}, {
	Name:  "invalid symbol/SK1",
	Data:  fmt.Sprintf("%q", sk1KeyStr[0:len(sk1KeyStr)-1]+"0"),
	Err:   "invalid format: version and/or checksum bytes missing",
	ID:    new(SK1Key),
	ExpID: new(SK1Key),
}, {
	Name:  "invalid checksum",
	Data:  fmt.Sprintf("%q", id1KeyStr[0:len(id1KeyStr)-1]+"e"),
	Err:   "checksum error",
	ID:    new(ID1Key),
	ExpID: new(ID1Key),
}, {
	Name:  "invalid checksum",
	Data:  fmt.Sprintf("%q", sk1KeyStr[0:len(sk1KeyStr)-1]+"e"),
	Err:   "checksum error",
	ID:    new(SK1Key),
	ExpID: new(SK1Key),
}, {
	Name:  "invalid symbol/ID2",
	Data:  fmt.Sprintf("%q", id2KeyStr[0:len(id2KeyStr)-1]+"0"),
	Err:   "invalid format: version and/or checksum bytes missing",
	ID:    new(ID2Key),
	ExpID: new(ID2Key),
}, {
	Name:  "invalid symbol/SK2",
	Data:  fmt.Sprintf("%q", sk2KeyStr[0:len(sk2KeyStr)-1]+"0"),
	Err:   "invalid format: version and/or checksum bytes missing",
	ID:    new(SK2Key),
	ExpID: new(SK2Key),
}, {
	Name:  "invalid checksum",
	Data:  fmt.Sprintf("%q", id2KeyStr[0:len(id2KeyStr)-1]+"e"),
	Err:   "checksum error",
	ID:    new(ID2Key),
	ExpID: new(ID2Key),
}, {
	Name:  "invalid checksum",
	Data:  fmt.Sprintf("%q", sk2KeyStr[0:len(sk2KeyStr)-1]+"e"),
	Err:   "checksum error",
	ID:    new(SK2Key),
	ExpID: new(SK2Key),
}, {
	Name:  "invalid symbol/ID3",
	Data:  fmt.Sprintf("%q", id3KeyStr[0:len(id3KeyStr)-1]+"0"),
	Err:   "invalid format: version and/or checksum bytes missing",
	ID:    new(ID3Key),
	ExpID: new(ID3Key),
}, {
	Name:  "invalid symbol/SK3",
	Data:  fmt.Sprintf("%q", sk3KeyStr[0:len(sk3KeyStr)-1]+"0"),
	Err:   "invalid format: version and/or checksum bytes missing",
	ID:    new(SK3Key),
	ExpID: new(SK3Key),
}, {
	Name:  "invalid checksum",
	Data:  fmt.Sprintf("%q", id3KeyStr[0:len(id3KeyStr)-1]+"e"),
	Err:   "checksum error",
	ID:    new(ID3Key),
	ExpID: new(ID3Key),
}, {
	Name:  "invalid checksum",
	Data:  fmt.Sprintf("%q", sk3KeyStr[0:len(sk3KeyStr)-1]+"e"),
	Err:   "checksum error",
	ID:    new(SK3Key),
	ExpID: new(SK3Key),
}, {
	Name:  "invalid symbol/ID4",
	Data:  fmt.Sprintf("%q", id4KeyStr[0:len(id4KeyStr)-1]+"0"),
	Err:   "invalid format: version and/or checksum bytes missing",
	ID:    new(ID4Key),
	ExpID: new(ID4Key),
}, {
	Name:  "invalid symbol/SK4",
	Data:  fmt.Sprintf("%q", sk4KeyStr[0:len(sk4KeyStr)-1]+"0"),
	Err:   "invalid format: version and/or checksum bytes missing",
	ID:    new(SK4Key),
	ExpID: new(SK4Key),
}, {
	Name:  "invalid checksum",
	Data:  fmt.Sprintf("%q", id4KeyStr[0:len(id4KeyStr)-1]+"e"),
	Err:   "checksum error",
	ID:    new(ID4Key),
	ExpID: new(ID4Key),
}, {
	Name:  "invalid checksum",
	Data:  fmt.Sprintf("%q", sk4KeyStr[0:len(sk4KeyStr)-1]+"e"),
	Err:   "checksum error",
	ID:    new(SK4Key),
	ExpID: new(SK4Key),
}}

func testIDKeyUnmarshalJSON(t *testing.T, test idKeyUnmarshalJSONTest) {
	err := json.Unmarshal([]byte(test.Data), test.ID)
	assert := assert.New(t)
	if len(test.Err) > 0 {
		assert.EqualError(err, test.Err)
		return
	}
	assert.NoError(err)
	assert.Equal(test.ExpID, test.ID)
}

func TestIDKey(t *testing.T) {
	for _, test := range idKeyUnmarshalJSONTests {
		if test.ID != nil {
			t.Run("UnmarshalJSON/"+test.Name, func(t *testing.T) {
				testIDKeyUnmarshalJSON(t, test)
			})
			continue
		}

		test.ExpID, test.ID = new(ID1Key), new(ID1Key)
		t.Run("UnmarshalJSON/"+test.Name+"/ID1Key", func(t *testing.T) {
			testIDKeyUnmarshalJSON(t, test)
		})
		test.ExpID, test.ID = new(SK1Key), new(SK1Key)
		t.Run("UnmarshalJSON/"+test.Name+"/SK1Key", func(t *testing.T) {
			testIDKeyUnmarshalJSON(t, test)
		})

		test.ExpID, test.ID = new(ID2Key), new(ID2Key)
		t.Run("UnmarshalJSON/"+test.Name+"/ID2Key", func(t *testing.T) {
			testIDKeyUnmarshalJSON(t, test)
		})
		test.ExpID, test.ID = new(SK2Key), new(SK2Key)
		t.Run("UnmarshalJSON/"+test.Name+"/SK2Key", func(t *testing.T) {
			testIDKeyUnmarshalJSON(t, test)
		})

		test.ExpID, test.ID = new(ID3Key), new(ID3Key)
		t.Run("UnmarshalJSON/"+test.Name+"/ID3Key", func(t *testing.T) {
			testIDKeyUnmarshalJSON(t, test)
		})
		test.ExpID, test.ID = new(SK3Key), new(SK3Key)
		t.Run("UnmarshalJSON/"+test.Name+"/SK3Key", func(t *testing.T) {
			testIDKeyUnmarshalJSON(t, test)
		})

		test.ExpID, test.ID = new(ID4Key), new(ID4Key)
		t.Run("UnmarshalJSON/"+test.Name+"/ID4Key", func(t *testing.T) {
			testIDKeyUnmarshalJSON(t, test)
		})
		test.ExpID, test.ID = new(SK4Key), new(SK4Key)
		t.Run("UnmarshalJSON/"+test.Name+"/SK4Key", func(t *testing.T) {
			testIDKeyUnmarshalJSON(t, test)
		})

	}

	id1, _ := NewID1Key(id1KeyStr)
	sk1, _ := NewSK1Key(sk1KeyStr)
	id2, _ := NewID2Key(id2KeyStr)
	sk2, _ := NewSK2Key(sk2KeyStr)
	id3, _ := NewID3Key(id3KeyStr)
	sk3, _ := NewSK3Key(sk3KeyStr)
	id4, _ := NewID4Key(id4KeyStr)
	sk4, _ := NewSK4Key(sk4KeyStr)

	strToKey := map[string]interface {
		PrefixString() string
		String() string
	}{
		id1KeyStr: id1, sk1KeyStr: sk1,
		id2KeyStr: id2, sk2KeyStr: sk2,
		id3KeyStr: id3, sk3KeyStr: sk3,
		id4KeyStr: id4, sk4KeyStr: sk4,
	}
	for keyStr, key := range strToKey {
		t.Run("MarshalJSON/"+key.PrefixString(), func(t *testing.T) {
			data, err := json.Marshal(key)
			assert := assert.New(t)
			assert.NoError(err)
			assert.Equal(fmt.Sprintf("%q", keyStr), string(data))
		})
		t.Run("String/"+key.PrefixString(), func(t *testing.T) {
			assert.Equal(t, keyStr, key.String())
		})
	}

	t.Run("SKKey/SK1", func(t *testing.T) {
		id, _ := NewID1Key(id1KeyStr)
		sk, _ := NewSK1Key(sk1KeyStr)
		assert := assert.New(t)
		assert.Equal(id, sk.ID1Key())
	})
	t.Run("SKKey/SK2", func(t *testing.T) {
		id, _ := NewID2Key(id2KeyStr)
		sk, _ := NewSK2Key(sk2KeyStr)
		assert := assert.New(t)
		assert.Equal(id, sk.ID2Key())
	})
	t.Run("SKKey/SK3", func(t *testing.T) {
		id, _ := NewID3Key(id3KeyStr)
		sk, _ := NewSK3Key(sk3KeyStr)
		assert := assert.New(t)
		assert.Equal(id, sk.ID3Key())
	})
	t.Run("SKKey/SK4", func(t *testing.T) {
		id, _ := NewID4Key(id4KeyStr)
		sk, _ := NewSK4Key(sk4KeyStr)
		assert := assert.New(t)
		assert.Equal(id, sk.ID4Key())
	})

	t.Run("Generate/SK1", func(t *testing.T) {
		_, err := GenerateSK1Key()
		assert.NoError(t, err)
	})
	t.Run("Generate/SK2", func(t *testing.T) {
		_, err := GenerateSK2Key()
		assert.NoError(t, err)
	})
	t.Run("Generate/SK3", func(t *testing.T) {
		_, err := GenerateSK3Key()
		assert.NoError(t, err)
	})
	t.Run("Generate/SK4", func(t *testing.T) {
		_, err := GenerateSK4Key()
		assert.NoError(t, err)
	})

}

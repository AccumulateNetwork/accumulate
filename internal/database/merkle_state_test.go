// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func TestMerkleState_Equal(t *testing.T) {
	var rh, rh1 common.RandHash
	ms1 := new(merkle.State)
	ms2 := new(merkle.State)
	for i := 0; i < 100; i++ {
		require.Truef(t, ms1.Equal(ms2), "Should be equal (%d)", i)
		ms1.Pending = append(ms1.Pending, nil)
		require.Truef(t, ms1.Equal(ms2), "Should be equal (%d)", i)
		ms2.Pending = append(ms2.Pending, nil, nil)
		require.Truef(t, ms1.Equal(ms2), "Should be equal (%d)", i)

		ms1.Add(rh.Next())

		require.Falsef(t, ms1.Equal(ms2), "Should be equal (%d)", i)
		ms1.Pending = append(ms1.Pending, nil)
		require.Falsef(t, ms1.Equal(ms2), "Should be equal (%d)", i)
		ms2.Pending = append(ms2.Pending, nil, nil)
		require.Falsef(t, ms1.Equal(ms2), "Should be equal (%d)", i)

		ms2.Add(rh1.Next())
		require.Truef(t, ms1.Equal(ms2), "Should be equal (%d)", i)
		ms1.Pending = append(ms1.Pending, nil)
		require.Truef(t, ms1.Equal(ms2), "Should be equal (%d)", i)
		ms2.Pending = append(ms2.Pending, nil, nil)
		require.Truef(t, ms1.Equal(ms2), "Should be equal (%d)", i)

	}
}

func TestCopy(t *testing.T) {
	ms1 := new(merkle.State)
	for i := 0; i < 15; i++ {
		hash := doSha([]byte(fmt.Sprintf("%x", i*i*i*i)))
		ms1.Add(hash)
	}
	ms1.Add(doSha([]byte{1, 2, 3, 4, 5}))
	ms2 := ms1
	if !ms1.Equal(ms2) {
		t.Error("ms1 should be equal ms2")
	}
	ms2 = ms1.Copy()
	ms3 := ms2.Copy()
	if !ms1.Equal(ms2) || !ms2.Equal(ms3) || !ms1.Equal(ms3) {
		t.Error("ms1 ms2 and ms3 should all be equal")
	}

	ms1.Add(doSha([]byte{1, 2, 3, 4, 5}))
	if ms1.Equal(ms2) {
		t.Error("ms1 should not equal ms2")
	}
	//fmt.Println(ms1.String())
	//fmt.Println(ms2.String())
}

func TestUnmarshalMemorySafety(t *testing.T) {
	// Create a Merkle state and add entries
	MS1 := new(merkle.State)
	for i := 0; i < 10; i++ {
		MS1.Add(doSha([]byte(fmt.Sprintf("%8d", i))))
	}

	// Marshal and unmarshal into a new state
	data, err := MS1.Marshal()
	require.NoError(t, err)
	MS2 := new(merkle.State)
	require.NoError(t, MS2.UnMarshal(data))

	// Overwrite the data array with garbage
	_, _ = rand.Read(data)

	// Ensure that MS2 did not change
	require.True(t, MS1.Equal(MS2), "States do not match")
}

func TestMarshal(t *testing.T) {
	MS1 := new(merkle.State)
	MS2 := new(merkle.State)

	data1, err := MS1.Marshal()
	if err != nil {
		t.Fatal("marshal should not fail")
	}
	require.NoError(t, MS2.UnMarshal(data1))
	data2, err2 := MS2.Marshal()
	if err2 != nil {
		t.Fatal("marshal should not fail")
	}

	if !bytes.Equal(data1, data2) {
		t.Error("Should be the same")
	}

	MS1.Add(doSha([]byte{1, 2, 3, 4, 5}))

	data1, err = MS1.Marshal()
	if err != nil {
		t.Fatal("marshal should not fail")
	}

	require.NoError(t, MS2.UnMarshal(data1))
	data2, err = MS2.Marshal()
	if err != nil {
		t.Fatal("marshal should not fail")
	}

	if !MS1.Equal(MS2) {
		t.Error("Should be the same")
	}
	if !bytes.Equal(data1, data2) {
		t.Error("Should be the same")
	}

	for i := 0; i < 10; i++ {
		MS1.Add(doSha([]byte(fmt.Sprintf("%8d", i))))

		data1, err = MS1.Marshal()
		if err != nil {
			t.Fatal("marshal should not fail")
		}
		require.NoError(t, MS2.UnMarshal(data1))
		data2, err = MS2.Marshal()
		if err != nil {
			t.Fatal("marshal should not fail")
		}
		if !bytes.Equal(data1, data2) {
			t.Error("Should be the same")
		}
		if !bytes.Equal(data1, data2) {
			t.Error("Should be the same")
		}
	}
}

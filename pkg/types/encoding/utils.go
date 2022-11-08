// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"fmt"
	"math/big"
	"reflect"
	"time"
)

func SplitDuration(d time.Duration) (sec, ns uint64) {
	sec = uint64(d.Seconds())
	ns = uint64((d - d.Round(time.Second)).Nanoseconds())
	return sec, ns
}

func BytesCopy(v []byte) []byte {
	u := make([]byte, len(v))
	copy(u, v)
	return v
}

func BigintCopy(v *big.Int) *big.Int {
	u := new(big.Int)
	u.Set(v)
	return u
}

// SetPtr sets *target = value
func SetPtr(value, target interface{}) (err error) {
	if value == nil {
		panic("value is nil")
	}
	if target == nil {
		panic("target is nil")
	}

	// Make sure target is a non-nil pointer
	rtarget := reflect.ValueOf(target)
	if rtarget.Kind() != reflect.Ptr {
		panic(fmt.Errorf("target %T is not a pointer", target))
	}
	if rtarget.IsNil() {
		panic("target is nil")
	}
	rtarget = rtarget.Elem()

	// If target is a pointer to value, there's nothing to do
	rvalue := reflect.ValueOf(value)
	if rvalue.Kind() == reflect.Ptr && rtarget.Kind() == reflect.Ptr && rvalue.Pointer() == rtarget.Pointer() {
		return nil
	}

	// Check if we can: *target = value
	if !rvalue.Type().AssignableTo(rtarget.Type()) {
		return fmt.Errorf("cannot assign %T to %v", value, rtarget.Type())
	}

	rtarget.Set(rvalue)
	return nil
}

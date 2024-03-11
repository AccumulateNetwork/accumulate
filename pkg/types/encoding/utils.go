// Copyright 2024 The Accumulate Authors
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

// SetPtr sets *target = value. If value cannot be assigned to *target and value
// has an Unwrap() method that returns a single value, SetPtr will retry with
// Unwrap()'s return value, recursively.
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

	// Check if value == *target
	rvalue := reflect.ValueOf(value)
try_assign:
	if rvalue.Kind() == reflect.Ptr && rtarget.Kind() == reflect.Ptr && rvalue.Pointer() == rtarget.Pointer() {
		return nil // Nothing to do
	}

	// Check if we can: *target = value
	if rvalue.Type().AssignableTo(rtarget.Type()) {
		rtarget.Set(rvalue)
		return nil
	}

	// Look for an Unwrap() method
	unwrap, ok := rvalue.Type().MethodByName("Unwrap")
	if !ok || unwrap.Type.NumIn() != 1 || unwrap.Type.NumOut() != 1 {
		return fmt.Errorf("cannot assign %T to %v", value, rtarget.Type())
	}

	// Unwrap and try again
	rvalue = unwrap.Func.Call([]reflect.Value{rvalue})[0]
	for rvalue.Kind() == reflect.Interface {
		rvalue = rvalue.Elem()
	}
	goto try_assign
}

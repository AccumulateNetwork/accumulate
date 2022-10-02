// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/golangci/golangci-lint/pkg/lint/linter"
	"github.com/golangci/golangci-lint/pkg/lint/lintersdb"
)

var customLinters []*linter.Config

func addCustomLinters(db *lintersdb.Manager) {
	field, ok := reflect.TypeOf(db).Elem().FieldByName("nameToLCs")
	if !ok {
		panic(fmt.Errorf("can't find linter config field"))
	}

	// This is a horrific abuse of Go. But using a plugin would be a huge PITA.
	nameToLCs := *(*map[string][]*linter.Config)(unsafe.Pointer(uintptr(unsafe.Pointer(db)) + field.Offset))

	for _, lc := range customLinters {
		nameToLCs[lc.Name()] = append(nameToLCs[lc.Name()], lc)
	}
}

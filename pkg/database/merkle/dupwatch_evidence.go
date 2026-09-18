// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

// EVIDENCE INSTRUMENTATION - NOT A FIX.
//
// This file exists to answer one question: does production code ever offer the
// same hash twice to the same chain, i.e. is AddEntry's dedup branch ever
// taken? It is inert unless ACC_DUP_LOG names a file.

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
)

var dupLog struct {
	sync.Mutex
	f    *os.File
	seen map[string]bool
}

func init() {
	name := os.Getenv("ACC_DUP_LOG")
	if name == "" {
		return
	}
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	dupLog.f = f
	dupLog.seen = map[string]bool{}
}

// recordDuplicate is called when AddEntry is given a hash that is already in
// the chain's element index.
func recordDuplicate(c *Chain, unique bool, at uint64) {
	if dupLog.f == nil {
		return
	}

	var pc [64]uintptr
	n := runtime.Callers(3, pc[:])
	frames := runtime.CallersFrames(pc[:n])
	var names []string
	for {
		fr, more := frames.Next()
		if strings.Contains(fr.Function, ".Test") {
			names = append(names, fr.Function[strings.LastIndex(fr.Function, "/")+1:])
			break
		}
		if strings.Contains(fr.File, "accumulate") && !strings.Contains(fr.File, "pkg/database/merkle") {
			fn := fr.Function
			if i := strings.LastIndex(fn, "/"); i >= 0 {
				fn = fn[i+1:]
			}
			names = append(names, fmt.Sprintf("%s:%d", fn, fr.Line))
		}
		if !more || len(names) >= 40 {
			break
		}
	}

	line := fmt.Sprintf("unique=%v chain=%s stack=%s\n",
		unique, c.name, strings.Join(names, " <- "))

	dupLog.Lock()
	defer dupLog.Unlock()
	if dupLog.seen[line] {
		return
	}
	dupLog.seen[line] = true
	_, _ = dupLog.f.WriteString(line)
}

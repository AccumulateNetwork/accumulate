package testutil

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"

	"github.com/google/pprof/profile"
)

// Copied from https://www.storj.io/blog/finding-goroutine-leaks-in-tests

func TrackGoroutines(ctx context.Context, t testing.TB, fn func(context.Context)) {
	err := track(ctx, t.Name(), fn)
	if err != nil {
		t.Fatal("Leaked goroutines\n", err)
	}
}

func track(ctx context.Context, name string, fn func(context.Context)) error {
	pprof.Do(ctx, pprof.Labels("test", name), fn)
	runtime.Gosched()
	return checkNoGoroutines("test", name)
}

func checkNoGoroutines(key, value string) error {
	var pb bytes.Buffer
	profiler := pprof.Lookup("goroutine")
	if profiler == nil {
		return fmt.Errorf("unable to find profile")
	}
	err := profiler.WriteTo(&pb, 0)
	if err != nil {
		return fmt.Errorf("unable to read profile: %w", err)
	}

	p, err := profile.ParseData(pb.Bytes())
	if err != nil {
		return fmt.Errorf("unable to parse profile: %w", err)
	}

	return summarizeGoroutines(p, key, value)
}

func summarizeGoroutines(p *profile.Profile, key, expectedValue string) error {
	var err LeakedGoroutines
	for _, sample := range p.Sample {
		if matchesLabel(sample, key, expectedValue) {
			err.Goroutine = append(err.Goroutine, sample)
		}
	}

	if len(err.Goroutine) == 0 {
		return nil
	}

	return &err
}

func matchesLabel(sample *profile.Sample, key, expectedValue string) bool {
	values, hasLabel := sample.Label[key]
	if !hasLabel {
		return false
	}

	for _, value := range values {
		if value == expectedValue {
			return true
		}
	}

	return false
}

type LeakedGoroutines struct {
	Goroutine []*profile.Sample
}

func (l *LeakedGoroutines) Error() string {
	var b strings.Builder

	for _, sample := range l.Goroutine {
		fmt.Fprintf(&b, "count %d @\n", sample.Value[0])
		// format the stack trace for each goroutine
		for _, loc := range sample.Location {
			for i, ln := range loc.Line {
				if i == 0 {
					fmt.Fprintf(&b, "#   %#8x", loc.Address)
					if loc.IsFolded {
						fmt.Fprint(&b, " [F]")
					}
				} else {
					fmt.Fprint(&b, "#           ")
				}
				if fn := ln.Function; fn != nil {
					fmt.Fprintf(&b, " %-50s %s:%d", fn.Name, fn.Filename, ln.Line)
				} else {
					fmt.Fprintf(&b, " ???")
				}
				fmt.Fprintf(&b, "\n")
			}
		}
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

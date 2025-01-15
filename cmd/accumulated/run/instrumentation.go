// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func (i *Instrumentation) start(inst *Instance) error {
	err := i.startPprof(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("pprof: %w", err)
	}

	err = i.listen(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("listen: %w", err)
	}

	setDefaultVal(&i.Monitoring, new(Monitor))
	err = i.Monitoring.start(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("monitoring: %w", err)
	}

	return nil
}

func (i *Instrumentation) listen(inst *Instance) error {
	// If there's no listening address, there's nothing to do. Someone else
	// (e.g. Tendermint) may be setting up a listener but we're not so we're
	// done.
	//
	// TODO Consider adding a default listening address.
	if i == nil || len(i.Listen) == 0 {
		return nil
	}
	i.applyHttpDefaults()
	_, err := i.startHTTP(inst, promhttp.InstrumentMetricHandler(
		prometheus.DefaultRegisterer, promhttp.HandlerFor(
			prometheus.DefaultGatherer,
			promhttp.HandlerOpts{MaxRequestsInFlight: int(*i.ConnectionLimit)},
		),
	))
	return err
}

func (i *Instrumentation) startPprof(inst *Instance) error {
	if i.PprofListen == nil {
		return nil
	}

	l, secure, err := httpListen(i.PprofListen)
	if err != nil {
		return err
	}
	if secure {
		return errors.BadRequest.With("https pprof not supported")
	}

	// Default HTTP server plus slow-loris prevention
	s := &http.Server{ReadHeaderTimeout: time.Minute}

	inst.run(func() {
		err := s.Serve(l)
		slog.Error("Server stopped (pprof)", "error", err)
	})

	inst.cleanup(s.Shutdown)
	return nil
}

func (m *Monitor) start(inst *Instance) error {
	if m == nil {
		m = new(Monitor)
	}

	setDefaultPtr(&m.ProfileMemory, false)                              // Enabled      = false
	setDefaultPtr(&m.MemoryPollingRate, encoding.Duration(time.Minute)) // Polling rate = every minute
	setDefaultPtr(&m.AllocRateTrigger, 50<<20)                          // Trigger rate = 50 MiB/s
	setDefaultVal(&m.Directory, "traces")                               // Directory    = ./traces

	if *m.ProfileMemory {
		err := os.MkdirAll(inst.path(m.Directory), 0700)
		if err != nil {
			return err
		}

		go m.pollMemory(inst)
	}

	return nil
}

func (m *Monitor) pollMemory(inst *Instance) {
	tick := time.NewTicker(m.MemoryPollingRate.Get())
	inst.cleanup(func(context.Context) error { tick.Stop(); return nil })

	var s1, s2 runtime.MemStats
	runtime.ReadMemStats(&s1)
	t1 := time.Now()

	for t2 := range tick.C {
		runtime.ReadMemStats(&s2)
		rate := (float64(s2.Alloc) - float64(s1.Alloc)) / t2.Sub(t1).Seconds()
		s1, t1 = s2, t2
		if rate < *m.AllocRateTrigger {
			continue
		}

		// TODO Capture at a higher frequency, regardless of allocation rate,
		// for some period after the spike

		m.takeHeapProfile(inst)
	}
}

func (m *Monitor) takeHeapProfile(inst *Instance) {
	time := time.Now().Format("2006-01-02-15-04-05.999")
	name := inst.path(m.Directory, fmt.Sprintf("memory-%s.pb.gz", time))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		slog.Error("Failed to open file for heap profile", "error", err, "path", name)
		return
	}
	defer func() {
		err = f.Close()
		if err != nil {
			slog.Error("Failed to close file after writing heap profile", "error", err, "path", name)
		}
	}()

	err = pprof.Lookup("heap").WriteTo(f, 0)
	if err != nil {
		slog.Error("Failed to capture heap profile", "error", err, "path", name)
	}
}

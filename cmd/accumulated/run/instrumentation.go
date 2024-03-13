package run

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (i *Instrumentation) start(inst *Instance) error {
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

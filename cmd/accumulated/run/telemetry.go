// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/exp/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	promex "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func (t *Telemetry) start(inst *Instance) error {
	if !setDefaultPtr(&t.Enabled, true) {
		return nil
	}

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Error(err.Error(), "module", "telemetry")
	}))

	res, err := resource.New(
		inst.context,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("accumulated"),
			semconv.ServiceVersionKey.String(accumulate.Version),
			semconv.ServiceInstanceIDKey.String(inst.id)),
		resource.WithTelemetrySDK(), // Discover and provide information about the OpenTelemetry SDK used.
		resource.WithProcess(),      // Discover and provide process information.
		resource.WithOS(),           // Discover and provide OS information.
		resource.WithContainer(),    // Discover and provide container information.
		resource.WithHost(),         // Discover and provide host information.
	)
	switch {
	case errors.Is(err, resource.ErrPartialResource):
		slog.Info("Partial resource", "error", err, "module", "telemetry")
	case err != nil:
		return err
	}

	return errors.Join(
		t.setupPropagator(res, inst),
		t.setupTraceProvider(res, inst),
		t.setupMeterProvider(res, inst),
		t.setupLoggerProvider(res, inst),
	)
}

func (t *Telemetry) setupPropagator(*resource.Resource, *Instance) error {
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
	return nil
}

func (t *Telemetry) setupTraceProvider(res *resource.Resource, inst *Instance) error {
	exporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint())
	if err != nil {
		return err
	}

	provider := trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithBatcher(exporter,
			// Default is 5s. Set to 1s for demonstrative purposes.
			trace.WithBatchTimeout(time.Second)),
	)

	inst.cleanup(provider.Shutdown)
	otel.SetTracerProvider(provider)
	return nil
}

func (t *Telemetry) setupMeterProvider(res *resource.Resource, inst *Instance) error {
	otelProm := &telemetry.OtelPromProducer{Gatherer: prometheus.DefaultGatherer}
	options := []metric.Option{
		metric.WithResource(res),
	}

	if t.Export != nil {
		registerer := prometheus.NewRegistry()
		exporter, err := promex.New(
			promex.WithoutScopeInfo(), // The HTTP handler breaks without this
			promex.WithRegisterer(registerer),
			promex.WithProducer(otelProm))
		if err != nil {
			return err
		}

		options = append(options,
			metric.WithReader(exporter))

		t.Export.applyHttpDefaults()
		_, err = t.Export.startHTTP(inst, promhttp.InstrumentMetricHandler(registerer,
			promhttp.HandlerFor(noErrGatherer{registerer},
				promhttp.HandlerOpts{MaxRequestsInFlight: int(*t.Export.ConnectionLimit)})))
		if err != nil {
			return err
		}
	}

	var exporters []metric.Exporter
	if setDefaultPtr(&t.Stdout, false) {
		exporter, err := stdoutmetric.New()
		if err != nil {
			return err
		}
		exporters = append(exporters, exporter)
	}

	if t.Otlp != nil && setDefaultPtr(&t.Otlp.Enabled, true) {
		auth := base64.StdEncoding.EncodeToString([]byte(t.Otlp.Username + ":" + t.Otlp.Password))
		endpoint := strings.TrimSuffix(t.Otlp.Endpoint, "/")
		exporter, err := otlpmetrichttp.New(inst.context,
			otlpmetrichttp.WithEndpointURL(endpoint+"/v1/metrics"),
			otlpmetrichttp.WithHeaders(map[string]string{"Authorization": "Basic " + auth}))
		if err != nil {
			return err
		}
		exporters = append(exporters, exporter)
	}

	var defaultRule *TelemetryRule
	for _, r := range t.Rules {
		if len(r.Match) > 0 {
			continue
		}
		if defaultRule != nil {
			return fmt.Errorf("cannot have multiple default rules")
		}
		defaultRule = r
	}
	if defaultRule == nil {
		defaultRule = new(TelemetryRule)
	}
	if !defaultRule.Drop && defaultRule.Rate == 0 {
		defaultRule.Rate.Set(time.Minute)
	}

	var drop []func(string) bool
	for _, r := range t.Rules {
		if !r.Drop || len(r.Match) == 0 {
			continue
		}
		f, err := r.compile()
		if err != nil {
			return err
		}
		drop = append(drop, f)
	}

	var rateFilters []func(string) bool
	var rates []time.Duration
	for _, r := range t.Rules {
		if r.Rate == 0 || len(r.Match) == 0 {
			continue
		}
		f, err := r.compile()
		if err != nil {
			return err
		}
		rateFilters = append(rateFilters, f)
		rates = append(rates, r.Rate.Get())
	}

	for _, exporter := range exporters {
		if !defaultRule.Drop {
			match := func(s string) bool {
				// Drop it?
				for _, drop := range drop {
					if drop(s) {
						return false
					}
				}

				// Is there a specific rule for this metric?
				for _, filter := range rateFilters {
					if filter(s) {
						return false
					}
				}

				return true
			}

			options = append(options,
				metric.WithReader(
					metric.NewPeriodicReader(&filterExporter{Exporter: exporter, match: match},
						metric.WithProducer(otelProm),
						metric.WithInterval(defaultRule.Rate.Get()))))
		}

		for i, rate := range rates {
			match := func(s string) bool {
				// Check the filter
				if !rateFilters[i](s) {
					return false
				}

				// Drop it?
				for _, drop := range drop {
					if drop(s) {
						return false
					}
				}

				return true
			}

			options = append(options,
				metric.WithReader(
					metric.NewPeriodicReader(&filterExporter{Exporter: exporter, match: match},
						metric.WithProducer(otelProm),
						metric.WithInterval(rate))))
		}
	}

	provider := metric.NewMeterProvider(options...)
	inst.cleanup(provider.Shutdown)
	otel.SetMeterProvider(provider)
	return nil
}

func (t *Telemetry) setupLoggerProvider(res *resource.Resource, inst *Instance) error {
	exporter, err := stdoutlog.New()
	if err != nil {
		return err
	}

	provider := log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(log.NewBatchProcessor(exporter)),
	)

	inst.cleanup(provider.Shutdown)
	global.SetLoggerProvider(provider)
	return nil
}

func (r *TelemetryRule) compile() (func(string) bool, error) {
	if len(r.Match) == 0 {
		return nil, nil
	}

	var exprs []*regexp.Regexp
	for _, s := range r.Match {
		expr, err := regexp.Compile("^(?:" + s + ")$")
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, expr)
	}

	return func(s string) bool {
		for _, expr := range exprs {
			if expr.MatchString(s) {
				return true
			}
		}
		return false
	}, nil
}

type filterExporter struct {
	metric.Exporter
	match func(string) bool
}

func (f *filterExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	var fsm []metricdata.ScopeMetrics
	for _, sm := range rm.ScopeMetrics {
		var fm []metricdata.Metrics
		for _, m := range sm.Metrics {
			if f.match(m.Name) {
				fm = append(fm, m)
			}
		}
		if len(fm) > 0 {
			sm.Metrics = fm
			fsm = append(fsm, sm)
		}
	}
	rm.ScopeMetrics = fsm
	return f.Exporter.Export(ctx, rm)
}

type noErrGatherer struct{ prometheus.Gatherer }

func (g noErrGatherer) Gather() ([]*dto.MetricFamily, error) {
	mfs, _ := g.Gatherer.Gather()
	return mfs, nil
}

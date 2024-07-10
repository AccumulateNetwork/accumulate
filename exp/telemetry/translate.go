// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import "strings"

const (
	// MetricMetadataTypeKey is the key used to store the original Prometheus
	// type in metric metadata:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/e6eccba97ebaffbbfad6d4358408a2cead0ec2df/specification/compatibility/prometheus_and_openmetrics.md#metric-metadata
	MetricMetadataTypeKey = "prometheus.type"
	// ExemplarTraceIDKey is the key used to store the trace ID in Prometheus
	// exemplars:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/e6eccba97ebaffbbfad6d4358408a2cead0ec2df/specification/compatibility/prometheus_and_openmetrics.md#exemplars
	ExemplarTraceIDKey = "trace_id"
	// ExemplarSpanIDKey is the key used to store the Span ID in Prometheus
	// exemplars:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/e6eccba97ebaffbbfad6d4358408a2cead0ec2df/specification/compatibility/prometheus_and_openmetrics.md#exemplars
	ExemplarSpanIDKey = "span_id"
	// ScopeInfoMetricName is the name of the metric used to preserve scope
	// attributes in Prometheus format:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/e6eccba97ebaffbbfad6d4358408a2cead0ec2df/specification/compatibility/prometheus_and_openmetrics.md#instrumentation-scope
	ScopeInfoMetricName = "otel_scope_info"
	// ScopeNameLabelKey is the name of the label key used to identify the name
	// of the OpenTelemetry scope which produced the metric:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/e6eccba97ebaffbbfad6d4358408a2cead0ec2df/specification/compatibility/prometheus_and_openmetrics.md#instrumentation-scope
	ScopeNameLabelKey = "otel_scope_name"
	// ScopeVersionLabelKey is the name of the label key used to identify the
	// version of the OpenTelemetry scope which produced the metric:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/e6eccba97ebaffbbfad6d4358408a2cead0ec2df/specification/compatibility/prometheus_and_openmetrics.md#instrumentation-scope
	ScopeVersionLabelKey = "otel_scope_version"
	// TargetInfoMetricName is the name of the metric used to preserve resource
	// attributes in Prometheus format:
	// https://github.com/open-telemetry/opentelemetry-specification/blob/e6eccba97ebaffbbfad6d4358408a2cead0ec2df/specification/compatibility/prometheus_and_openmetrics.md#resource-attributes-1
	// It originates from OpenMetrics:
	// https://github.com/OpenObservability/OpenMetrics/blob/1386544931307dff279688f332890c31b6c5de36/specification/OpenMetrics.md#supporting-target-metadata-in-both-push-based-and-pull-based-systems
	TargetInfoMetricName = "target_info"
)

var wordToUCUM = map[string]string{

	// Time
	"days":         "d",
	"hours":        "h",
	"minutes":      "min",
	"seconds":      "s",
	"milliseconds": "ms",
	"microseconds": "us",
	"nanoseconds":  "ns",

	// Bytes
	"bytes":     "By",
	"kibibytes": "KiBy",
	"mebibytes": "MiBy",
	"gibibytes": "GiBy",
	"tibibytes": "TiBy",
	"kilobytes": "KBy",
	"megabytes": "MBy",
	"gigabytes": "GBy",
	"terabytes": "TBy",

	// SI
	"meters":  "m",
	"volts":   "V",
	"amperes": "A",
	"joules":  "J",
	"watts":   "W",
	"grams":   "g",

	// Misc
	"celsius": "Cel",
	"hertz":   "Hz",
	"ratio":   "1",
	"percent": "%",
}

// The map that translates the "per" unit
// Example: per_second (singular) => /s
var perWordToUCUM = map[string]string{
	"second": "s",
	"minute": "m",
	"hour":   "h",
	"day":    "d",
	"week":   "w",
	"month":  "mo",
	"year":   "y",
}

// unitWordToUCUM converts english unit words to UCUM units:
// https://ucum.org/ucum#section-Alphabetic-Index-By-Symbol
// It also handles rates, such as meters_per_second, by translating the first
// word to UCUM, and the "per" word to UCUM. It joins them with a "/" between.
func unitWordToUCUM(unit string) string {
	unitTokens := strings.SplitN(unit, "_per_", 2)
	if len(unitTokens) == 0 {
		return ""
	}
	ucumUnit := wordToUCUMOrDefault(unitTokens[0])
	if len(unitTokens) > 1 && unitTokens[1] != "" {
		ucumUnit += "/" + perWordToUCUMOrDefault(unitTokens[1])
	}
	return ucumUnit
}

// wordToUCUMOrDefault retrieves the Prometheus "basic" unit corresponding to
// the specified "basic" unit. Returns the specified unit if not found in
// wordToUCUM.
func wordToUCUMOrDefault(unit string) string {
	if promUnit, ok := wordToUCUM[unit]; ok {
		return promUnit
	}
	return unit
}

// perWordToUCUMOrDefault retrieve the Prometheus "per" unit corresponding to
// the specified "per" unit. Returns the specified unit if not found in perWordToUCUM.
func perWordToUCUMOrDefault(perUnit string) string {
	if promPerUnit, ok := perWordToUCUM[perUnit]; ok {
		return promPerUnit
	}
	return perUnit
}

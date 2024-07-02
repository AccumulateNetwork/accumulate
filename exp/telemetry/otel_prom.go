// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package telemetry

import (
	"context"
	"encoding/hex"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"gitlab.com/accumulatenetwork/accumulate"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var reConsensus = regexp.MustCompile(`^(?:cometbft|consensus|tendermint)_([A-Za-z\d]+)(?:_(\d+))_((?:abci|blocksync|consensus|mempool|p2p|state)_.*)`)

type OtelPromProducer struct {
	Network  string
	Gatherer prometheus.Gatherer
}

func (p *OtelPromProducer) Produce(ctx context.Context) ([]metricdata.ScopeMetrics, error) {
	prom, err := p.Gatherer.Gather()
	if err != nil {
		return nil, err
	}

	// Conversion based on https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/11396751f11905fe62df105f444cef9ab531def0/receiver/prometheusreceiver/internal/metricfamily.go

	var otel []metricdata.ScopeMetrics
	for _, fProm := range prom {
		var fOtel metricdata.ScopeMetrics

		// Set the scope
		for _, m := range fProm.Metric {
			for _, l := range m.Label {
				if l.GetName() == ScopeNameLabelKey {
					fOtel.Scope.Name = l.GetValue()
				}
				if l.GetName() == ScopeVersionLabelKey {
					fOtel.Scope.Version = l.GetValue()
				}
			}
		}
		if fOtel.Scope.Name == "" {
			fOtel.Scope.Name = "accumulate/otelcol/prometheusproducer"
		}
		if fOtel.Scope.Version == "" {
			fOtel.Scope.Version = accumulate.Version
		}

		// TODO: Trim suffixes? Prometheus type?
		var mOtel metricdata.Metrics
		mOtel.Name = fProm.GetName()
		mOtel.Description = fProm.GetHelp()
		mOtel.Unit = unitWordToUCUM(fProm.GetUnit())

		// Transform cometbft_ and consensus_ to tendermint_ and remove the
		// partition. For example:
		//
		// consensus_chico_consensus_total_txs ->
		// tendermint_chico_consensus_total_txs ->
		// tendermint_consensus_total_txs{partition="chico"}
		var attrs []attribute.KeyValue
		if p.Network != "" {
			// TODO: Replace with a proper, pipelined label rewriter that also
			// works for native otel metrics
			attrs = append(attrs,
				attribute.String("network", p.Network))
		}
		if m := reConsensus.FindStringSubmatch(mOtel.Name); m != nil {
			attrs = append(attrs,
				attribute.String("partition", m[1]))
			if m[2] != "" {
				attrs = append(attrs,
					attribute.String("node", m[2]))
			}
			mOtel.Name = "tendermint_" + m[3]
		}

		for _, mProm := range fProm.Metric {
			mOtel := mOtel
			points := getDataPoints(mProm, attrs)

			switch fProm.GetType() {
			case dto.MetricType_COUNTER:
				mOtel.Data = metricdata.Sum[float64]{}
				continue

			case dto.MetricType_SUMMARY:
				mOtel.Data = metricdata.Summary{}
				continue

			case dto.MetricType_HISTOGRAM:
				mOtel.Data = metricdata.Histogram[float64]{}
				continue

			default: // gauge, untyped, gauge histogram
				mOtel.Data = metricdata.Gauge[float64]{DataPoints: points}
			}

			fOtel.Metrics = append(fOtel.Metrics, mOtel)
		}

		if len(fOtel.Metrics) > 0 {
			otel = append(otel, fOtel)
		}
	}
	return otel, nil
}

var notUsefulLabels = func() []string {
	s := []string{model.MetricNameLabel, model.InstanceLabel, model.SchemeLabel, model.MetricsPathLabel, model.JobLabel, model.BucketLabel, model.QuantileLabel}
	sort.Strings(s)
	return s
}()

func getDataPoints(metric *dto.Metric, extraAttrs []attribute.KeyValue) []metricdata.DataPoint[float64] {
	attributes := attribute.NewSet(append(attrsFromLabels(metric.Label), extraAttrs...)...)
	timestamp := timeFromMs(metric)
	var startTime time.Time
	switch {
	case metric.Counter != nil:
		startTime = timeFromPb(metric.Counter.CreatedTimestamp)
	case metric.Summary != nil:
		startTime = timeFromPb(metric.Summary.CreatedTimestamp)
	case metric.Histogram != nil:
		startTime = timeFromPb(metric.Histogram.CreatedTimestamp)
	}
	if startTime == (time.Time{}) {
		startTime = timestamp
	}

	var points []metricdata.DataPoint[float64]
	if metric.Gauge != nil && metric.Gauge.Value != nil {
		points = append(points, metricdata.DataPoint[float64]{
			StartTime:  startTime,
			Time:       timestamp,
			Attributes: attributes,
			Value:      metric.Gauge.GetValue(),
		})
	}

	if metric.Counter != nil && metric.Counter.Value != nil {
		points = append(points, metricdata.DataPoint[float64]{
			StartTime:  startTime,
			Time:       timestamp,
			Attributes: attributes,
			Value:      metric.Counter.GetValue(),
			Exemplars:  maybeConvertExemplar(metric.Counter.Exemplar),
		})
	}

	// TODO: Summary

	if metric.Untyped != nil && metric.Untyped.Value != nil {
		points = append(points, metricdata.DataPoint[float64]{
			StartTime:  startTime,
			Time:       timestamp,
			Attributes: attributes,
			Value:      metric.Untyped.GetValue(),
		})
	}

	// TODO: Histogram

	return points
}

func maybeConvertExemplar(prom *dto.Exemplar) []metricdata.Exemplar[float64] {
	if prom == nil {
		return nil
	}
	return []metricdata.Exemplar[float64]{convertExemplar(prom)}
}

func convertExemplar(prom *dto.Exemplar) metricdata.Exemplar[float64] {
	var otel metricdata.Exemplar[float64]
	otel.Value = prom.GetValue()
	otel.Time = timeFromPb(prom.Timestamp)

	for _, l := range prom.Label {
		switch strings.ToLower(l.GetName()) {
		case ExemplarTraceIDKey:
			var tid [16]byte
			err := decodeAndCopyToLowerBytes(tid[:], []byte(l.GetValue()))
			if err == nil {
				otel.TraceID = tid[:]
				continue
			}
		case ExemplarSpanIDKey:
			var sid [8]byte
			err := decodeAndCopyToLowerBytes(sid[:], []byte(l.GetValue()))
			if err == nil {
				otel.SpanID = sid[:]
				continue
			}
		}

		otel.FilteredAttributes = append(otel.FilteredAttributes,
			attribute.String(l.GetName(), l.GetValue()))
	}

	return otel
}

func timeFromMs(v interface{ GetTimestampMs() int64 }) time.Time {
	ts := v.GetTimestampMs()
	if ts == 0 {
		// This isn't a real solution, but Grafana will reject the push if any
		// timestamps are empty
		return time.Now()
	}
	return time.UnixMilli(ts)
}

func timeFromPb(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return time.Unix(ts.Seconds, int64(ts.Nanos))
}

func attrsFromLabels(labels []*dto.LabelPair) []attribute.KeyValue {
	slices.SortFunc(labels, func(a, b *dto.LabelPair) int {
		return strings.Compare(a.GetName(), b.GetName())
	})

	var attrs []attribute.KeyValue
	j := 0
	for _, l := range labels {
		for j < len(notUsefulLabels) && notUsefulLabels[j] < l.GetName() {
			j++
		}
		if j < len(notUsefulLabels) && l.GetName() == notUsefulLabels[j] {
			continue
		}
		if l.GetValue() == "" {
			// empty label values should be omitted
			continue
		}
		attrs = append(attrs, attribute.String(l.GetName(), l.GetValue()))
	}
	return attrs
}

/*
	decodeAndCopyToLowerBytes copies src to dst on lower bytes instead of higher

1. If len(src) > len(dst) -> copy first len(dst) bytes as it is. Example -> src = []byte{0xab,0xcd,0xef,0xgh,0xij}, dst = [2]byte, result dst = [2]byte{0xab, 0xcd}
2. If len(src) = len(dst) -> copy src to dst as it is
3. If len(src) < len(dst) -> prepend required 0s and then add src to dst. Example -> src = []byte{0xab, 0xcd}, dst = [8]byte, result dst = [8]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xab, 0xcd}
*/
func decodeAndCopyToLowerBytes(dst []byte, src []byte) error {
	var err error
	decodedLen := hex.DecodedLen(len(src))
	if decodedLen >= len(dst) {
		_, err = hex.Decode(dst, src[:hex.EncodedLen(len(dst))])
	} else {
		_, err = hex.Decode(dst[len(dst)-decodedLen:], src)
	}
	return err
}

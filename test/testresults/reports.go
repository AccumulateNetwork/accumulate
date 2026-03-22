// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testresults

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"
)

// ReportFormat defines the output format for reports
type ReportFormat string

const (
	FormatMarkdown ReportFormat = "markdown"
	FormatJSON     ReportFormat = "json"
	FormatHTML     ReportFormat = "html"
)

// FormatSingleRunReport generates a report for a single test run
func FormatSingleRunReport(run *TestRun, format ReportFormat) (string, error) {
	switch format {
	case FormatMarkdown:
		return formatSingleRunMarkdown(run), nil
	case FormatJSON:
		return run.ToJSON()
	case FormatHTML:
		return formatSingleRunHTML(run)
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}
}

// FormatComparisonReport generates a comparison report
func FormatComparisonReport(comparison *ComparisonResult, format ReportFormat) (string, error) {
	switch format {
	case FormatMarkdown:
		return formatComparisonMarkdown(comparison), nil
	case FormatJSON:
		data, err := json.MarshalIndent(comparison, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	case FormatHTML:
		return formatComparisonHTML(comparison)
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}
}

// FormatTrendReport generates a trend analysis report
func FormatTrendReport(trend *TrendAnalysis, format ReportFormat) (string, error) {
	switch format {
	case FormatMarkdown:
		return formatTrendMarkdown(trend), nil
	case FormatJSON:
		data, err := json.MarshalIndent(trend, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	case FormatHTML:
		return formatTrendHTML(trend)
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}
}

func formatSingleRunMarkdown(run *TestRun) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("# Test Run Report - ID %d\n\n", run.ID))
	buf.WriteString(fmt.Sprintf("**Commit:** %s\n", run.CommitHash))
	buf.WriteString(fmt.Sprintf("**Branch:** %s\n", run.Branch))
	buf.WriteString(fmt.Sprintf("**Started:** %s\n", run.StartTime.Format(time.RFC3339)))
	buf.WriteString(fmt.Sprintf("**Duration:** %d seconds\n\n", run.Duration))

	buf.WriteString("## Configuration\n\n")
	buf.WriteString(fmt.Sprintf("- **Target TPS:** %d\n", run.TargetTPS))
	buf.WriteString(fmt.Sprintf("- **Concurrency:** %d\n", run.Concurrency))
	buf.WriteString("\n")

	buf.WriteString("## Results\n\n")
	buf.WriteString(fmt.Sprintf("- **Total Transactions:** %d\n", run.TotalTx))
	buf.WriteString(fmt.Sprintf("- **Passed:** %d (%.2f%%)\n", run.TxPassed, float64(run.TxPassed)/float64(run.TotalTx)*100))
	buf.WriteString(fmt.Sprintf("- **Failed:** %d (%.2f%%)\n", run.TxFailed, float64(run.TxFailed)/float64(run.TotalTx)*100))
	buf.WriteString("\n")

	buf.WriteString("## Performance Metrics\n\n")
	buf.WriteString("### Throughput\n\n")
	buf.WriteString(fmt.Sprintf("- **Average TPS:** %.2f\n", run.AverageTPS))
	buf.WriteString(fmt.Sprintf("- **Peak TPS:** %.2f\n", run.PeakTPS))
	buf.WriteString("\n")

	buf.WriteString("### Latency (ms)\n\n")
	buf.WriteString(fmt.Sprintf("- **Min:** %.2f\n", run.MinLatency))
	buf.WriteString(fmt.Sprintf("- **Avg:** %.2f\n", run.AvgLatency))
	buf.WriteString(fmt.Sprintf("- **Max:** %.2f\n", run.MaxLatency))
	buf.WriteString(fmt.Sprintf("- **P50:** %.2f\n", run.P50Latency))
	buf.WriteString(fmt.Sprintf("- **P95:** %.2f\n", run.P95Latency))
	buf.WriteString(fmt.Sprintf("- **P99:** %.2f\n", run.P99Latency))
	buf.WriteString("\n")

	buf.WriteString("### Stability\n\n")
	buf.WriteString(fmt.Sprintf("- **Error Rate:** %.2f%%\n", run.ErrorRate))
	buf.WriteString(fmt.Sprintf("- **Node Crashes:** %d\n", run.NodeCrashes))
	buf.WriteString(fmt.Sprintf("- **Node Restarts:** %d\n", run.NodeRestarts))
	buf.WriteString("\n")

	if run.Notes != "" {
		buf.WriteString("## Notes\n\n")
		buf.WriteString(run.Notes)
		buf.WriteString("\n")
	}

	return buf.String()
}

func formatComparisonMarkdown(comparison *ComparisonResult) string {
	var buf bytes.Buffer

	buf.WriteString("# Test Run Comparison Report\n\n")
	buf.WriteString(comparison.Summary)
	buf.WriteString("\n\n")

	buf.WriteString("## Base Run\n\n")
	buf.WriteString(fmt.Sprintf("- **ID:** %d\n", comparison.BaseRun.ID))
	buf.WriteString(fmt.Sprintf("- **Commit:** %s\n", comparison.BaseRun.CommitHash))
	buf.WriteString(fmt.Sprintf("- **Date:** %s\n", comparison.BaseRun.StartTime.Format(time.RFC3339)))
	buf.WriteString("\n")

	buf.WriteString("## Compare Run\n\n")
	buf.WriteString(fmt.Sprintf("- **ID:** %d\n", comparison.CompareRun.ID))
	buf.WriteString(fmt.Sprintf("- **Commit:** %s\n", comparison.CompareRun.CommitHash))
	buf.WriteString(fmt.Sprintf("- **Date:** %s\n", comparison.CompareRun.StartTime.Format(time.RFC3339)))
	buf.WriteString("\n")

	if len(comparison.Regressions) > 0 {
		buf.WriteString("## ⚠️  Regressions\n\n")
		for _, reg := range comparison.Regressions {
			buf.WriteString(fmt.Sprintf("- %s\n", reg))
		}
		buf.WriteString("\n")
	}

	if len(comparison.Improvements) > 0 {
		buf.WriteString("## ✓ Improvements\n\n")
		for _, imp := range comparison.Improvements {
			buf.WriteString(fmt.Sprintf("- %s\n", imp))
		}
		buf.WriteString("\n")
	}

	buf.WriteString("## Detailed Metrics Comparison\n\n")
	buf.WriteString("| Metric | Base | Compare | Change | % Change |\n")
	buf.WriteString("|--------|------|---------|--------|----------|\n")

	metrics := []string{"avg_tps", "peak_tps", "avg_latency", "p95_latency", "p99_latency", "error_rate"}
	for _, key := range metrics {
		if comp, ok := comparison.Metrics[key]; ok {
			icon := ""
			if comp.IsRegression {
				icon = "⚠️ "
			} else if comp.PercentChange > 5 || (strings.Contains(key, "latency") && comp.PercentChange < -5) {
				icon = "✓ "
			}

			buf.WriteString(fmt.Sprintf("| %s%s | %.2f | %.2f | %+.2f | %+.2f%% |\n",
				icon, comp.MetricName, comp.BaseValue, comp.CompareValue,
				comp.Delta, comp.PercentChange))
		}
	}
	buf.WriteString("\n")

	return buf.String()
}

func formatTrendMarkdown(trend *TrendAnalysis) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("# Trend Analysis: %s\n\n", trend.MetricName))
	buf.WriteString(fmt.Sprintf("**Trend:** %s\n", trend.Trend))
	buf.WriteString(fmt.Sprintf("**Runs Analyzed:** %d\n", len(trend.Runs)))
	buf.WriteString(fmt.Sprintf("**Time Range:** %s to %s\n\n",
		trend.Timestamps[0].Format("2006-01-02"),
		trend.Timestamps[len(trend.Timestamps)-1].Format("2006-01-02")))

	buf.WriteString("## Values Over Time\n\n")
	buf.WriteString("| Date | Commit | Value |\n")
	buf.WriteString("|------|--------|-------|\n")

	for i, run := range trend.Runs {
		buf.WriteString(fmt.Sprintf("| %s | %s | %.2f |\n",
			run.StartTime.Format("2006-01-02 15:04"),
			run.CommitHash[:8],
			trend.Values[i]))
	}
	buf.WriteString("\n")

	// Simple ASCII chart
	buf.WriteString("## Trend Visualization\n\n")
	buf.WriteString("```\n")
	buf.WriteString(generateASCIIChart(trend.Values))
	buf.WriteString("```\n\n")

	return buf.String()
}

func generateASCIIChart(values []float64) string {
	if len(values) == 0 {
		return ""
	}

	// Find min and max
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	// Normalize to 0-20 range for chart height
	height := 20
	var buf bytes.Buffer

	for row := height; row >= 0; row-- {
		threshold := min + (max-min)*float64(row)/float64(height)
		for _, v := range values {
			if v >= threshold {
				buf.WriteString("█")
			} else {
				buf.WriteString(" ")
			}
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

func formatSingleRunHTML(run *TestRun) (string, error) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Test Run Report - ID {{.ID}}</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 1200px; margin: 40px auto; padding: 20px; }
        h1, h2, h3 { color: #333; }
        .metric { display: inline-block; margin: 10px; padding: 15px; background: #f5f5f5; border-radius: 5px; }
        .metric-value { font-size: 24px; font-weight: bold; color: #0066cc; }
        .metric-label { font-size: 12px; color: #666; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #f5f5f5; }
    </style>
</head>
<body>
    <h1>Test Run Report - ID {{.ID}}</h1>
    <p><strong>Commit:</strong> {{.CommitHash}}</p>
    <p><strong>Branch:</strong> {{.Branch}}</p>
    <p><strong>Started:</strong> {{.StartTime.Format "2006-01-02 15:04:05"}}</p>
    <p><strong>Duration:</strong> {{.Duration}} seconds</p>

    <h2>Performance Metrics</h2>
    <div class="metric">
        <div class="metric-value">{{printf "%.2f" .AverageTPS}}</div>
        <div class="metric-label">Average TPS</div>
    </div>
    <div class="metric">
        <div class="metric-value">{{printf "%.2f" .PeakTPS}}</div>
        <div class="metric-label">Peak TPS</div>
    </div>
    <div class="metric">
        <div class="metric-value">{{printf "%.2f" .AvgLatency}}</div>
        <div class="metric-label">Avg Latency (ms)</div>
    </div>
    <div class="metric">
        <div class="metric-value">{{printf "%.2f" .P95Latency}}</div>
        <div class="metric-label">P95 Latency (ms)</div>
    </div>
    <div class="metric">
        <div class="metric-value">{{printf "%.2f" .ErrorRate}}%</div>
        <div class="metric-label">Error Rate</div>
    </div>

    <h2>Results</h2>
    <table>
        <tr><th>Metric</th><th>Value</th></tr>
        <tr><td>Total Transactions</td><td>{{.TotalTx}}</td></tr>
        <tr><td>Passed</td><td>{{.TxPassed}}</td></tr>
        <tr><td>Failed</td><td>{{.TxFailed}}</td></tr>
        <tr><td>Node Crashes</td><td>{{.NodeCrashes}}</td></tr>
        <tr><td>Node Restarts</td><td>{{.NodeRestarts}}</td></tr>
    </table>
</body>
</html>`

	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, run); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func formatComparisonHTML(comparison *ComparisonResult) (string, error) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Test Run Comparison</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 1200px; margin: 40px auto; padding: 20px; }
        h1, h2 { color: #333; }
        .summary { padding: 20px; margin: 20px 0; border-radius: 5px; font-weight: bold; }
        .regression { background: #ffebee; color: #c62828; }
        .improvement { background: #e8f5e9; color: #2e7d32; }
        .neutral { background: #f5f5f5; color: #666; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #f5f5f5; }
        .reg-icon { color: #c62828; }
        .imp-icon { color: #2e7d32; }
    </style>
</head>
<body>
    <h1>Test Run Comparison Report</h1>
    <div class="summary {{if gt (len .Regressions) 0}}regression{{else if gt (len .Improvements) 0}}improvement{{else}}neutral{{end}}">
        {{.Summary}}
    </div>

    <h2>Runs Being Compared</h2>
    <table>
        <tr>
            <th></th>
            <th>ID</th>
            <th>Commit</th>
            <th>Date</th>
        </tr>
        <tr>
            <td><strong>Base</strong></td>
            <td>{{.BaseRun.ID}}</td>
            <td>{{.BaseRun.CommitHash}}</td>
            <td>{{.BaseRun.StartTime.Format "2006-01-02 15:04"}}</td>
        </tr>
        <tr>
            <td><strong>Compare</strong></td>
            <td>{{.CompareRun.ID}}</td>
            <td>{{.CompareRun.CommitHash}}</td>
            <td>{{.CompareRun.StartTime.Format "2006-01-02 15:04"}}</td>
        </tr>
    </table>

    {{if gt (len .Regressions) 0}}
    <h2 class="reg-icon">⚠️ Regressions</h2>
    <ul>
        {{range .Regressions}}<li>{{.}}</li>{{end}}
    </ul>
    {{end}}

    {{if gt (len .Improvements) 0}}
    <h2 class="imp-icon">✓ Improvements</h2>
    <ul>
        {{range .Improvements}}<li>{{.}}</li>{{end}}
    </ul>
    {{end}}

    <h2>Detailed Metrics</h2>
    <table>
        <tr>
            <th>Metric</th>
            <th>Base</th>
            <th>Compare</th>
            <th>Change</th>
            <th>% Change</th>
        </tr>
        {{range $key, $comp := .Metrics}}
        <tr>
            <td>{{if $comp.IsRegression}}<span class="reg-icon">⚠️</span> {{end}}{{$comp.MetricName}}</td>
            <td>{{printf "%.2f" $comp.BaseValue}}</td>
            <td>{{printf "%.2f" $comp.CompareValue}}</td>
            <td>{{printf "%+.2f" $comp.Delta}}</td>
            <td>{{printf "%+.2f" $comp.PercentChange}}%</td>
        </tr>
        {{end}}
    </table>
</body>
</html>`

	t, err := template.New("comparison").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, comparison); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func formatTrendHTML(trend *TrendAnalysis) (string, error) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Trend Analysis - {{.MetricName}}</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 1200px; margin: 40px auto; padding: 20px; }
        h1, h2 { color: #333; }
        .trend { padding: 15px; margin: 20px 0; border-radius: 5px; font-weight: bold; }
        .improving { background: #e8f5e9; color: #2e7d32; }
        .degrading { background: #ffebee; color: #c62828; }
        .stable { background: #f5f5f5; color: #666; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #f5f5f5; }
    </style>
</head>
<body>
    <h1>Trend Analysis: {{.MetricName}}</h1>
    <div class="trend {{.Trend}}">
        Trend: {{.Trend}}
    </div>
    <p><strong>Runs Analyzed:</strong> {{len .Runs}}</p>

    <h2>Values Over Time</h2>
    <table>
        <tr>
            <th>Date</th>
            <th>Commit</th>
            <th>Value</th>
        </tr>
        {{range $i, $run := .Runs}}
        <tr>
            <td>{{$run.StartTime.Format "2006-01-02 15:04"}}</td>
            <td>{{slice $run.CommitHash 0 8}}</td>
            <td>{{index $.Values $i | printf "%.2f"}}</td>
        </tr>
        {{end}}
    </table>
</body>
</html>`

	t, err := template.New("trend").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, trend); err != nil {
		return "", err
	}

	return buf.String(), nil
}

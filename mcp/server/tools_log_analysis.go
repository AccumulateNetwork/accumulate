package server

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// LogAnalysisResult contains structured log analysis results
type LogAnalysisResult struct {
	ContainerName   string            `json:"container_name"`
	LinesAnalyzed   int               `json:"lines_analyzed"`
	ErrorCount      int               `json:"error_count"`
	WarningCount    int               `json:"warning_count"`
	CriticalCount   int               `json:"critical_count"`
	ErrorSummary    map[string]int    `json:"error_summary"`    // Error type -> count
	RecentErrors    []LogEntry        `json:"recent_errors"`    // Last N errors
	RecentWarnings  []LogEntry        `json:"recent_warnings"`  // Last N warnings
	Patterns        []PatternMatch    `json:"patterns"`         // Detected patterns
	HealthIndicators map[string]string `json:"health_indicators"`
	Recommendations []string          `json:"recommendations"`
	Status          string            `json:"status"` // healthy, degraded, critical
	Message         string            `json:"message"`
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp string `json:"timestamp,omitempty"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Context   string `json:"context,omitempty"`
}

// PatternMatch represents a detected log pattern
type PatternMatch struct {
	Pattern     string `json:"pattern"`
	Count       int    `json:"count"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // info, warning, error, critical
}

// analyzeLogs analyzes follower container logs for errors and patterns
func (s *Server) analyzeLogs(args map[string]interface{}) (map[string]interface{}, error) {
	containerName, _ := args["container_name"].(string)
	if containerName == "" {
		containerName = "accumulate-follower"
	}

	// Validate container name to prevent command injection
	if err := ValidateContainerName(containerName); err != nil {
		return nil, fmt.Errorf("invalid container_name: %w", err)
	}

	linesFloat, _ := args["lines"].(float64)
	lines := int(linesFloat)
	if lines == 0 {
		lines = 500
	}

	// Validate lines parameter (prevent excessive resource usage)
	if lines > 10000 {
		lines = 10000
	}

	filter, _ := args["filter"].(string)
	if filter == "" {
		filter = "all"
	}

	result := &LogAnalysisResult{
		ContainerName:    containerName,
		ErrorSummary:     make(map[string]int),
		RecentErrors:     []LogEntry{},
		RecentWarnings:   []LogEntry{},
		Patterns:         []PatternMatch{},
		HealthIndicators: make(map[string]string),
		Recommendations:  []string{},
		Status:           "healthy",
	}

	// Check if container exists
	exists, err := containerExists(containerName)
	if err != nil || !exists {
		result.Status = "unknown"
		result.Message = fmt.Sprintf("Container '%s' not found", containerName)
		return structToMap(result)
	}

	// Get container logs with configurable timeout
	timeout := s.state.Config.LogAnalysisTimeout
	if timeout == 0 {
		timeout = 30 * time.Second // Fallback default
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("Failed to get container logs: %v", err)
		return structToMap(result)
	}

	logLines := strings.Split(string(output), "\n")
	result.LinesAnalyzed = len(logLines)

	// Define patterns to look for
	patterns := []struct {
		regex       *regexp.Regexp
		description string
		severity    string
		category    string
	}{
		{regexp.MustCompile(`(?i)panic`), "Application panic", "critical", "panic"},
		{regexp.MustCompile(`(?i)fatal`), "Fatal error", "critical", "fatal"},
		{regexp.MustCompile(`(?i)error.*database|database.*error`), "Database error", "error", "database"},
		{regexp.MustCompile(`(?i)error.*connection|connection.*error|connection refused`), "Connection error", "error", "connection"},
		{regexp.MustCompile(`(?i)error.*timeout|timeout.*error`), "Timeout error", "error", "timeout"},
		{regexp.MustCompile(`(?i)error.*consensus|consensus.*error`), "Consensus error", "error", "consensus"},
		{regexp.MustCompile(`(?i)error.*signature|signature.*error|invalid signature`), "Signature error", "error", "signature"},
		{regexp.MustCompile(`(?i)no peers|0 peers|peer count: 0`), "No peers connected", "warning", "peers"},
		{regexp.MustCompile(`(?i)sync.*stall|stall.*sync`), "Sync stalled", "warning", "sync"},
		{regexp.MustCompile(`(?i)disk.*full|no space|ENOSPC`), "Disk full", "critical", "disk"},
		{regexp.MustCompile(`(?i)out of memory|OOM|cannot allocate`), "Out of memory", "critical", "memory"},
		{regexp.MustCompile(`(?i)error`), "General error", "error", "general"},
		{regexp.MustCompile(`(?i)warn`), "Warning", "warning", "warning"},
	}

	// Pattern match counts
	patternCounts := make(map[string]int)

	// Analyze each line
	for _, line := range logLines {
		if line == "" {
			continue
		}

		entry := parseLogLine(line)

		// Check each pattern
		for _, p := range patterns {
			if p.regex.MatchString(line) {
				patternCounts[p.category]++

				switch p.severity {
				case "critical":
					result.CriticalCount++
					if len(result.RecentErrors) < 10 {
						entry.Level = "critical"
						result.RecentErrors = append(result.RecentErrors, entry)
					}
				case "error":
					if p.category != "general" || !containsAnyPattern(line, []string{"panic", "fatal", "database", "connection", "timeout", "consensus", "signature"}) {
						result.ErrorCount++
						result.ErrorSummary[p.category]++
						if len(result.RecentErrors) < 10 {
							entry.Level = "error"
							result.RecentErrors = append(result.RecentErrors, entry)
						}
					}
				case "warning":
					if p.category != "warning" || !containsAnyPattern(line, []string{"peers", "sync"}) {
						result.WarningCount++
						if len(result.RecentWarnings) < 10 {
							entry.Level = "warning"
							result.RecentWarnings = append(result.RecentWarnings, entry)
						}
					}
				}
				break // Only count each line once for the most specific pattern
			}
		}
	}

	// Build pattern summary
	for category, count := range patternCounts {
		if count > 0 {
			var desc, sev string
			switch category {
			case "panic":
				desc = "Application panics detected"
				sev = "critical"
			case "fatal":
				desc = "Fatal errors detected"
				sev = "critical"
			case "database":
				desc = "Database-related errors"
				sev = "error"
			case "connection":
				desc = "Network connection issues"
				sev = "error"
			case "timeout":
				desc = "Operation timeouts"
				sev = "error"
			case "consensus":
				desc = "Consensus-related errors"
				sev = "error"
			case "signature":
				desc = "Signature verification failures"
				sev = "error"
			case "peers":
				desc = "Peer connectivity issues"
				sev = "warning"
			case "sync":
				desc = "Synchronization issues"
				sev = "warning"
			case "disk":
				desc = "Disk space issues"
				sev = "critical"
			case "memory":
				desc = "Memory issues"
				sev = "critical"
			default:
				desc = "General issues"
				sev = "info"
			}

			result.Patterns = append(result.Patterns, PatternMatch{
				Pattern:     category,
				Count:       count,
				Description: desc,
				Severity:    sev,
			})
		}
	}

	// Sort patterns by severity and count
	sort.Slice(result.Patterns, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "error": 1, "warning": 2, "info": 3}
		if sevOrder[result.Patterns[i].Severity] != sevOrder[result.Patterns[j].Severity] {
			return sevOrder[result.Patterns[i].Severity] < sevOrder[result.Patterns[j].Severity]
		}
		return result.Patterns[i].Count > result.Patterns[j].Count
	})

	// Set health indicators
	if result.CriticalCount > 0 {
		result.HealthIndicators["critical_errors"] = fmt.Sprintf("%d critical errors", result.CriticalCount)
	}
	if result.ErrorCount > 0 {
		result.HealthIndicators["errors"] = fmt.Sprintf("%d errors", result.ErrorCount)
	}
	if result.WarningCount > 0 {
		result.HealthIndicators["warnings"] = fmt.Sprintf("%d warnings", result.WarningCount)
	}
	if patternCounts["peers"] > 0 {
		result.HealthIndicators["peer_issues"] = "Peer connectivity problems detected"
	}

	// Determine overall status
	if result.CriticalCount > 0 {
		result.Status = "critical"
		result.Message = fmt.Sprintf("Critical issues detected: %d critical, %d errors, %d warnings",
			result.CriticalCount, result.ErrorCount, result.WarningCount)
	} else if result.ErrorCount > 5 {
		result.Status = "degraded"
		result.Message = fmt.Sprintf("Multiple errors detected: %d errors, %d warnings",
			result.ErrorCount, result.WarningCount)
	} else if result.ErrorCount > 0 || result.WarningCount > 10 {
		result.Status = "degraded"
		result.Message = fmt.Sprintf("Some issues detected: %d errors, %d warnings",
			result.ErrorCount, result.WarningCount)
	} else {
		result.Status = "healthy"
		result.Message = fmt.Sprintf("No significant issues: %d warnings in %d lines analyzed",
			result.WarningCount, result.LinesAnalyzed)
	}

	// Generate recommendations
	result.Recommendations = generateRecommendations(result)

	// Filter results if requested
	if filter == "error" {
		result.RecentWarnings = nil
	} else if filter == "warning" {
		result.RecentErrors = nil
	} else if filter == "critical" {
		// Keep only critical in errors
		var criticalOnly []LogEntry
		for _, e := range result.RecentErrors {
			if e.Level == "critical" {
				criticalOnly = append(criticalOnly, e)
			}
		}
		result.RecentErrors = criticalOnly
		result.RecentWarnings = nil
	}

	return structToMap(result)
}

// parseLogLine parses a single log line into a LogEntry
func parseLogLine(line string) LogEntry {
	entry := LogEntry{
		Message: line,
	}

	// Try to extract timestamp (common formats)
	// Format: 2025-01-15T10:30:45.123Z
	tsRegex := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)`)
	if match := tsRegex.FindStringSubmatch(line); len(match) > 1 {
		entry.Timestamp = match[1]
		entry.Message = strings.TrimSpace(line[len(match[0]):])
	}

	// Try to extract log level
	levelRegex := regexp.MustCompile(`(?i)\b(DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|PANIC)\b`)
	if match := levelRegex.FindStringSubmatch(entry.Message); len(match) > 1 {
		entry.Level = strings.ToLower(match[1])
	}

	// Truncate very long messages
	if len(entry.Message) > 500 {
		entry.Message = entry.Message[:497] + "..."
	}

	return entry
}

// containsAnyPattern checks if a string contains any of the given patterns
func containsAnyPattern(s string, patterns []string) bool {
	lower := strings.ToLower(s)
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// generateRecommendations generates actionable recommendations based on analysis
func generateRecommendations(result *LogAnalysisResult) []string {
	var recs []string

	for _, pattern := range result.Patterns {
		switch pattern.Pattern {
		case "panic":
			recs = append(recs, "Critical: Application panics detected. Consider restarting the follower and checking for version compatibility.")
		case "fatal":
			recs = append(recs, "Critical: Fatal errors detected. Check configuration and database integrity.")
		case "database":
			recs = append(recs, "Database errors detected. Verify database is not corrupted. Consider re-deploying from fresh snapshots if persistent.")
		case "connection":
			recs = append(recs, "Network connection issues detected. Check firewall settings and ensure ports 16591-16593, 16691-16693 are open.")
		case "timeout":
			recs = append(recs, "Timeout errors detected. Check network latency and consider increasing timeout configurations.")
		case "consensus":
			recs = append(recs, "Consensus errors detected. Ensure follower is using correct network configuration and compatible version.")
		case "peers":
			recs = append(recs, "Peer connectivity issues. Verify bootstrap peers are correct and reachable. Check firewall and network connectivity.")
		case "sync":
			recs = append(recs, "Sync stalled. Check peer connections and network health. May need to restart follower or re-deploy from newer snapshots.")
		case "disk":
			recs = append(recs, "Critical: Disk space issues. Free up disk space immediately or add more storage.")
		case "memory":
			recs = append(recs, "Critical: Memory issues. Consider increasing system memory or reducing other workloads.")
		}
	}

	// Add general recommendations
	if result.Status == "critical" {
		recs = append(recs, "Immediate attention required. Consider stopping the follower, investigating the issues, and restarting.")
	} else if result.Status == "degraded" {
		recs = append(recs, "Monitor the follower closely. If issues persist, consider restarting or re-deploying.")
	}

	// Deduplicate recommendations
	seen := make(map[string]bool)
	var unique []string
	for _, r := range recs {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}

	return unique
}

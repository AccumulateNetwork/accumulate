package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PrerequisitesResult contains the validation results for follower deployment
type PrerequisitesResult struct {
	Valid       bool                `json:"valid"`
	Checks      []PrerequisiteCheck `json:"checks"`
	Summary     string              `json:"summary"`
	Errors      []string            `json:"errors"`
	Warnings    []string            `json:"warnings"`
	Suggestions []string            `json:"suggestions"`
}

// PrerequisiteCheck represents a single prerequisite check
type PrerequisiteCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "fail", "warn"
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// validatePrerequisites validates all system prerequisites for follower deployment
func (s *Server) validatePrerequisites(args map[string]interface{}) (map[string]interface{}, error) {
	workDir, _ := args["work_dir"].(string)
	network, _ := args["network"].(string)
	if network == "" {
		network = "mainnet"
	}

	result := &PrerequisitesResult{
		Valid:       true,
		Checks:      []PrerequisiteCheck{},
		Errors:      []string{},
		Warnings:    []string{},
		Suggestions: []string{},
	}

	// 1. Check disk space
	diskCheck := checkDiskSpace(workDir)
	result.Checks = append(result.Checks, diskCheck)
	if diskCheck.Status == "fail" {
		result.Valid = false
		result.Errors = append(result.Errors, diskCheck.Message)
	} else if diskCheck.Status == "warn" {
		result.Warnings = append(result.Warnings, diskCheck.Message)
	}

	// 2. Check available memory
	memCheck := checkMemory()
	result.Checks = append(result.Checks, memCheck)
	if memCheck.Status == "fail" {
		result.Valid = false
		result.Errors = append(result.Errors, memCheck.Message)
	} else if memCheck.Status == "warn" {
		result.Warnings = append(result.Warnings, memCheck.Message)
	}

	// 3. Check Docker availability
	dockerCheck := checkDocker()
	result.Checks = append(result.Checks, dockerCheck)
	if dockerCheck.Status == "fail" {
		result.Valid = false
		result.Errors = append(result.Errors, dockerCheck.Message)
	}

	// 4. Check required ports
	portChecks := checkPorts()
	for _, check := range portChecks {
		result.Checks = append(result.Checks, check)
		if check.Status == "fail" {
			result.Valid = false
			result.Errors = append(result.Errors, check.Message)
		}
	}

	// 5. Check network connectivity to bootstrap server
	netCheck := checkNetworkConnectivity(network)
	result.Checks = append(result.Checks, netCheck)
	if netCheck.Status == "fail" {
		result.Valid = false
		result.Errors = append(result.Errors, netCheck.Message)
	} else if netCheck.Status == "warn" {
		result.Warnings = append(result.Warnings, netCheck.Message)
	}

	// 6. Check work directory permissions
	if workDir != "" {
		dirCheck := checkDirectoryPermissions(workDir)
		result.Checks = append(result.Checks, dirCheck)
		if dirCheck.Status == "fail" {
			result.Valid = false
			result.Errors = append(result.Errors, dirCheck.Message)
		}
	}

	// 7. Check CPU cores
	cpuCheck := checkCPU()
	result.Checks = append(result.Checks, cpuCheck)
	if cpuCheck.Status == "warn" {
		result.Warnings = append(result.Warnings, cpuCheck.Message)
	}

	// Generate summary
	passCount := 0
	failCount := 0
	warnCount := 0
	for _, check := range result.Checks {
		switch check.Status {
		case "pass":
			passCount++
		case "fail":
			failCount++
		case "warn":
			warnCount++
		}
	}

	if result.Valid {
		if warnCount > 0 {
			result.Summary = fmt.Sprintf("Prerequisites validated with warnings: %d passed, %d warnings", passCount, warnCount)
		} else {
			result.Summary = fmt.Sprintf("All prerequisites validated: %d checks passed", passCount)
		}
	} else {
		result.Summary = fmt.Sprintf("Prerequisites validation failed: %d passed, %d failed, %d warnings", passCount, failCount, warnCount)
	}

	// Add suggestions based on failures
	if !result.Valid {
		for _, err := range result.Errors {
			if strings.Contains(err, "disk space") {
				result.Suggestions = append(result.Suggestions, "Free up disk space or use a larger volume for the work directory")
			}
			if strings.Contains(err, "memory") {
				result.Suggestions = append(result.Suggestions, "Close other applications or increase system memory")
			}
			if strings.Contains(err, "Docker") {
				result.Suggestions = append(result.Suggestions, "Install Docker: https://docs.docker.com/engine/install/")
			}
			if strings.Contains(err, "port") {
				result.Suggestions = append(result.Suggestions, "Stop services using the required ports or configure port mapping")
			}
			if strings.Contains(err, "bootstrap") {
				result.Suggestions = append(result.Suggestions, "Check network connectivity and firewall settings")
			}
		}
	}

	return map[string]interface{}{
		"valid":       result.Valid,
		"checks":      result.Checks,
		"summary":     result.Summary,
		"errors":      result.Errors,
		"warnings":    result.Warnings,
		"suggestions": result.Suggestions,
	}, nil
}

func checkDiskSpace(workDir string) PrerequisiteCheck {
	check := PrerequisiteCheck{
		Name: "Disk Space",
	}

	// Determine path to check
	checkPath := workDir
	if checkPath == "" {
		checkPath = os.TempDir()
	}

	// Ensure path exists or use parent
	for checkPath != "" && checkPath != "/" {
		if _, err := os.Stat(checkPath); err == nil {
			break
		}
		checkPath = filepath.Dir(checkPath)
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(checkPath, &stat); err != nil {
		check.Status = "warn"
		check.Message = fmt.Sprintf("Could not check disk space: %v", err)
		return check
	}

	// Calculate available space in GB
	availableGB := float64(stat.Bavail*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	totalGB := float64(stat.Blocks*uint64(stat.Bsize)) / (1024 * 1024 * 1024)

	check.Details = fmt.Sprintf("%.1f GB available of %.1f GB total on %s", availableGB, totalGB, checkPath)

	if availableGB < 50 {
		check.Status = "fail"
		check.Message = fmt.Sprintf("Insufficient disk space: %.1f GB available, need at least 50 GB (100 GB recommended)", availableGB)
	} else if availableGB < 100 {
		check.Status = "warn"
		check.Message = fmt.Sprintf("Low disk space: %.1f GB available (100 GB recommended)", availableGB)
	} else {
		check.Status = "pass"
		check.Message = fmt.Sprintf("Sufficient disk space: %.1f GB available", availableGB)
	}

	return check
}

func checkMemory() PrerequisiteCheck {
	check := PrerequisiteCheck{
		Name: "System Memory",
	}

	// Read memory info from /proc/meminfo
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		check.Status = "warn"
		check.Message = fmt.Sprintf("Could not check memory: %v", err)
		return check
	}

	var totalKB, availableKB uint64
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB, _ = strconv.ParseUint(fields[1], 10, 64)
		case "MemAvailable:":
			availableKB, _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}

	totalGB := float64(totalKB) / (1024 * 1024)
	availableGB := float64(availableKB) / (1024 * 1024)

	check.Details = fmt.Sprintf("%.1f GB available of %.1f GB total", availableGB, totalGB)

	if totalGB < 4 {
		check.Status = "fail"
		check.Message = fmt.Sprintf("Insufficient total memory: %.1f GB (need at least 8 GB)", totalGB)
	} else if totalGB < 8 {
		check.Status = "warn"
		check.Message = fmt.Sprintf("Low total memory: %.1f GB (8 GB+ recommended)", totalGB)
	} else if availableGB < 2 {
		check.Status = "warn"
		check.Message = fmt.Sprintf("Low available memory: %.1f GB (consider closing other applications)", availableGB)
	} else {
		check.Status = "pass"
		check.Message = fmt.Sprintf("Sufficient memory: %.1f GB total, %.1f GB available", totalGB, availableGB)
	}

	return check
}

func checkDocker() PrerequisiteCheck {
	check := PrerequisiteCheck{
		Name: "Docker",
	}

	// Check if docker command exists
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		check.Status = "fail"
		check.Message = "Docker not found in PATH"
		check.Details = "Install Docker from https://docs.docker.com/engine/install/"
		return check
	}

	check.Details = fmt.Sprintf("Docker binary: %s", dockerPath)

	// Check if docker daemon is running
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		check.Status = "fail"
		check.Message = "Docker daemon not running"
		check.Details = fmt.Sprintf("Error: %v. Try: sudo systemctl start docker", err)
		return check
	}

	// Extract Docker version
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, " Server Version:") {
			check.Details = strings.TrimSpace(line)
			break
		}
	}

	// Check if we can pull images (optional - just check permissions)
	cmd = exec.CommandContext(ctx, "docker", "ps", "-q")
	if err := cmd.Run(); err != nil {
		check.Status = "warn"
		check.Message = "Docker running but may require sudo"
		check.Details = "Consider adding user to docker group: sudo usermod -aG docker $USER"
		return check
	}

	check.Status = "pass"
	check.Message = "Docker is installed and running"

	return check
}

func checkPorts() []PrerequisiteCheck {
	// Ports required for follower operation
	requiredPorts := []struct {
		port int
		desc string
	}{
		{16591, "DN Tendermint RPC"},
		{16592, "DN API"},
		{16593, "DN P2P"},
		{16691, "BVN Tendermint RPC"},
		{16692, "BVN API"},
		{16693, "BVN P2P"},
	}

	var checks []PrerequisiteCheck

	for _, p := range requiredPorts {
		check := PrerequisiteCheck{
			Name: fmt.Sprintf("Port %d (%s)", p.port, p.desc),
		}

		// Try to listen on the port
		addr := fmt.Sprintf(":%d", p.port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			check.Status = "fail"
			check.Message = fmt.Sprintf("Port %d is in use (%s)", p.port, p.desc)
			check.Details = fmt.Sprintf("Error: %v", err)
		} else {
			listener.Close()
			check.Status = "pass"
			check.Message = fmt.Sprintf("Port %d is available", p.port)
		}

		checks = append(checks, check)
	}

	return checks
}

func checkNetworkConnectivity(network string) PrerequisiteCheck {
	check := PrerequisiteCheck{
		Name: "Network Connectivity",
	}

	// Determine bootstrap server URL based on network
	var bootstrapURL string
	switch network {
	case "mainnet":
		bootstrapURL = "http://bootstrap.accumulate.defidevs.io:8080/health"
	case "testnet":
		bootstrapURL = "http://testnet-bootstrap.accumulate.defidevs.io:8080/health"
	default:
		bootstrapURL = "http://bootstrap.accumulate.defidevs.io:8080/health"
	}

	check.Details = fmt.Sprintf("Testing connectivity to %s", bootstrapURL)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(bootstrapURL)
	if err != nil {
		check.Status = "fail"
		check.Message = fmt.Sprintf("Cannot reach bootstrap server: %v", err)
		return check
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		check.Status = "pass"
		check.Message = "Bootstrap server is reachable"
	} else {
		check.Status = "warn"
		check.Message = fmt.Sprintf("Bootstrap server returned status %d", resp.StatusCode)
	}

	return check
}

func checkDirectoryPermissions(workDir string) PrerequisiteCheck {
	check := PrerequisiteCheck{
		Name: "Directory Permissions",
	}

	// Check if parent directory exists and is writable
	parentDir := filepath.Dir(workDir)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		// Try to find existing parent
		for parentDir != "/" && parentDir != "." {
			if _, err := os.Stat(parentDir); err == nil {
				break
			}
			parentDir = filepath.Dir(parentDir)
		}
	}

	check.Details = fmt.Sprintf("Checking write access to %s", parentDir)

	// Try to create a test file
	testFile := filepath.Join(parentDir, ".accumulate-write-test")
	f, err := os.Create(testFile)
	if err != nil {
		check.Status = "fail"
		check.Message = fmt.Sprintf("Cannot write to directory %s: %v", parentDir, err)
		return check
	}
	f.Close()
	os.Remove(testFile)

	// Check if work directory already exists
	if info, err := os.Stat(workDir); err == nil {
		if info.IsDir() {
			check.Status = "pass"
			check.Message = fmt.Sprintf("Work directory exists and is writable: %s", workDir)
		} else {
			check.Status = "fail"
			check.Message = fmt.Sprintf("Path exists but is not a directory: %s", workDir)
		}
	} else {
		check.Status = "pass"
		check.Message = fmt.Sprintf("Parent directory is writable, work directory will be created: %s", workDir)
	}

	return check
}

func checkCPU() PrerequisiteCheck {
	check := PrerequisiteCheck{
		Name: "CPU Cores",
	}

	numCPU := runtime.NumCPU()
	check.Details = fmt.Sprintf("%d logical CPU cores available", numCPU)

	if numCPU < 2 {
		check.Status = "fail"
		check.Message = fmt.Sprintf("Insufficient CPU cores: %d (need at least 2)", numCPU)
	} else if numCPU < 4 {
		check.Status = "warn"
		check.Message = fmt.Sprintf("Limited CPU cores: %d (4+ recommended)", numCPU)
	} else {
		check.Status = "pass"
		check.Message = fmt.Sprintf("Sufficient CPU cores: %d", numCPU)
	}

	return check
}

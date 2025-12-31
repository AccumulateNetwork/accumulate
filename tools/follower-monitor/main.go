package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Command-line flags
var (
	workDir      = flag.String("work-dir", "", "Follower work directory (contains dnn/, bvnn/, logs/)")
	artifactsDir = flag.String("artifacts-dir", "", "Artifacts directory with newer binaries for update detection")
	bindAddr     = flag.String("bind", "127.0.0.1", "Address to bind to (use 0.0.0.0 for all interfaces)")
	port         = flag.Int("port", 9999, "HTTP server port (default 9999, internal use only)")
	interval     = flag.Int("interval", 10, "Update interval in seconds")
)

// DeploymentState represents the current deployment state
type DeploymentState string

const (
	StateUnknown       DeploymentState = "unknown"
	StateNotDeployed   DeploymentState = "not_deployed"
	StateInitializingDN  DeploymentState = "initializing_dn"
	StateInitializingBVN DeploymentState = "initializing_bvn"
	StateStopped       DeploymentState = "stopped"
	StateStarting      DeploymentState = "starting"
	StateRunning       DeploymentState = "running"
	StateStopping      DeploymentState = "stopping"
)

type NodeMetrics struct {
	Name           string    `json:"name"`
	DNHeight       int64     `json:"dn_height"`
	BVNHeight      int64     `json:"bvn_height"`
	DNCatchingUp   bool      `json:"dn_catching_up"`
	BVNCatchingUp  bool      `json:"bvn_catching_up"`
	DNPeers        int       `json:"dn_peers"`
	BVNPeers       int       `json:"bvn_peers"`
	DNMoniker      string    `json:"dn_moniker"`
	BVNMoniker     string    `json:"bvn_moniker"`
	LastUpdate     time.Time `json:"last_update"`
	DNBlockRate    float64   `json:"dn_block_rate"`
	BVNBlockRate   float64   `json:"bvn_block_rate"`
	DNGenesisRate  float64   `json:"dn_genesis_rate"`
	BVNGenesisRate float64   `json:"bvn_genesis_rate"`
	DNWeeklyRate   float64   `json:"dn_weekly_rate"`
	BVNWeeklyRate  float64   `json:"bvn_weekly_rate"`
	DNSinceLaunch  float64   `json:"dn_since_launch"`
	BVNSinceLaunch float64   `json:"bvn_since_launch"`
	DNRollingRate  float64   `json:"dn_rolling_rate"`
	BVNRollingRate float64   `json:"bvn_rolling_rate"`
	Available      bool      `json:"available"`
	Error          string    `json:"error,omitempty"`
}

type ComparisonMetrics struct {
	DNLag          int64     `json:"dn_lag"`
	BVNLag         int64     `json:"bvn_lag"`
	DNSyncPercent  float64   `json:"dn_sync_percent"`
	BVNSyncPercent float64   `json:"bvn_sync_percent"`
	DNETA          string    `json:"dn_eta"`
	BVNETA         string    `json:"bvn_eta"`
	Alerts         []Alert   `json:"alerts"`
	LastCheck      time.Time `json:"last_check"`
}

type Alert struct {
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Source  string    `json:"source"`
}

type DeploymentStatus struct {
	State          DeploymentState `json:"state"`
	WorkDir        string          `json:"work_dir"`
	PID            int             `json:"pid,omitempty"`
	StartedAt      time.Time       `json:"started_at,omitempty"`
	Uptime         string          `json:"uptime,omitempty"`
	DeployedAt     time.Time       `json:"deployed_at,omitempty"`
	DeployedAge    string          `json:"deployed_age,omitempty"`
	DNInitialized  bool            `json:"dn_initialized"`
	BVNInitialized bool            `json:"bvn_initialized"`
	ConfigExists   bool            `json:"config_exists"`
	BinaryVersion  string          `json:"binary_version,omitempty"`
	BinaryAge      string          `json:"binary_age,omitempty"`
	UpdateAvailable bool           `json:"update_available"`
	RecentLogs     []LogEntry      `json:"recent_logs"`
}

type PortInfo struct {
	DNRPC    int `json:"dn_rpc"`
	DNP2P    int `json:"dn_p2p"`
	DNAPI    int `json:"dn_api"`
	BVNRPC   int `json:"bvn_rpc"`
	BVNP2P   int `json:"bvn_p2p"`
	BVNAPI   int `json:"bvn_api"`
}

type Monitor struct {
	mainnet      *NodeMetrics
	follower     *NodeMetrics
	comparison   *ComparisonMetrics
	deployment   *DeploymentStatus
	ports        *PortInfo
	mu           sync.RWMutex

	// For rate calculation
	lastMainnetDN   int64
	lastMainnetBVN  int64
	lastFollowerDN  int64
	lastFollowerBVN int64
	lastCheckTime   time.Time

	// Follower launch tracking
	followerLaunchTime      time.Time
	followerLaunchDNHeight  int64
	followerLaunchBVNHeight int64

	// Rolling window tracking
	oneHourStartTime time.Time
	oneHourDNHeight  int64
	oneHourBVNHeight int64
	twoHourStartTime time.Time
	twoHourDNHeight  int64
	twoHourBVNHeight int64

	// Log file positions for tailing
	logPositions map[string]int64
}

type CometStatus struct {
	Result struct {
		NodeInfo struct {
			Network string `json:"network"`
			Moniker string `json:"moniker"`
			ID      string `json:"id"`
		} `json:"node_info"`
		SyncInfo struct {
			LatestBlockHeight string `json:"latest_block_height"`
			LatestBlockTime   string `json:"latest_block_time"`
			CatchingUp        bool   `json:"catching_up"`
		} `json:"sync_info"`
	} `json:"result"`
}

type BlockInfo struct {
	Result struct {
		Block struct {
			Header struct {
				Height string `json:"height"`
				Time   string `json:"time"`
			} `json:"header"`
		} `json:"block"`
	} `json:"result"`
}

type NetInfo struct {
	Result struct {
		NPeers string `json:"n_peers"`
	} `json:"result"`
}

const (
	mainnetDNRPC   = "http://23.22.212.106:16592"
	mainnetBVNRPC  = "http://23.22.212.106:16692"
	followerDNRPC  = "http://localhost:16592"
	followerBVNRPC = "http://localhost:16692"
	maxLogLines    = 100
)

func NewMonitor() *Monitor {
	now := time.Now()
	return &Monitor{
		mainnet:            &NodeMetrics{Name: "apollo-mainnet"},
		follower:           &NodeMetrics{Name: "follower"},
		comparison:         &ComparisonMetrics{Alerts: []Alert{}},
		deployment:         &DeploymentStatus{State: StateUnknown, RecentLogs: []LogEntry{}},
		ports:              &PortInfo{DNRPC: 16592, DNP2P: 16591, DNAPI: 16595, BVNRPC: 16692, BVNP2P: 16691, BVNAPI: 16695},
		lastCheckTime:      now,
		followerLaunchTime: now,
		oneHourStartTime:   now,
		twoHourStartTime:   now,
		logPositions:       make(map[string]int64),
	}
}

func (m *Monitor) fetchNodeMetrics(node *NodeMetrics, dnRPC, bvnRPC string) {
	var wg sync.WaitGroup
	wg.Add(2)

	var dnErr, bvnErr error

	go func() {
		defer wg.Done()
		status, err := fetchStatus(dnRPC)
		if err != nil {
			dnErr = err
			return
		}
		height := parseInt64(status.Result.SyncInfo.LatestBlockHeight)
		node.DNHeight = height
		node.DNCatchingUp = status.Result.SyncInfo.CatchingUp
		node.DNMoniker = status.Result.NodeInfo.Moniker

		netInfo, err := fetchNetInfo(dnRPC)
		if err == nil {
			node.DNPeers = parseInt(netInfo.Result.NPeers)
		}
	}()

	go func() {
		defer wg.Done()
		status, err := fetchStatus(bvnRPC)
		if err != nil {
			bvnErr = err
			return
		}
		height := parseInt64(status.Result.SyncInfo.LatestBlockHeight)
		node.BVNHeight = height
		node.BVNCatchingUp = status.Result.SyncInfo.CatchingUp
		node.BVNMoniker = status.Result.NodeInfo.Moniker

		netInfo, err := fetchNetInfo(bvnRPC)
		if err == nil {
			node.BVNPeers = parseInt(netInfo.Result.NPeers)
		}
	}()

	wg.Wait()
	node.LastUpdate = time.Now()

	if dnErr != nil && bvnErr != nil {
		node.Available = false
		node.Error = fmt.Sprintf("DN: %v, BVN: %v", dnErr, bvnErr)
	} else {
		node.Available = true
		node.Error = ""
	}
}

func (m *Monitor) updateDeploymentStatus() {
	if *workDir == "" {
		m.deployment.State = StateUnknown
		m.deployment.WorkDir = "(not configured)"
		return
	}

	m.deployment.WorkDir = *workDir

	// Check if directories exist - look for data directories, not just config
	dnnDir := filepath.Join(*workDir, "dnn")
	bvnnDir := filepath.Join(*workDir, "bvnn")
	configPath := filepath.Join(*workDir, "accumulate.toml")

	// Check for DN initialization - look for data directory or config
	dnnDataStat, dnnDataErr := os.Stat(filepath.Join(dnnDir, "data"))
	_, dnnConfigErr := os.Stat(filepath.Join(dnnDir, "config"))
	m.deployment.DNInitialized = dnnDataErr == nil || dnnConfigErr == nil

	// Check for BVN initialization
	bvnnDataStat, bvnnDataErr := os.Stat(filepath.Join(bvnnDir, "data"))
	_, bvnnConfigErr := os.Stat(filepath.Join(bvnnDir, "config"))
	m.deployment.BVNInitialized = bvnnDataErr == nil || bvnnConfigErr == nil

	_, configErr := os.Stat(configPath)
	m.deployment.ConfigExists = configErr == nil

	// Estimate deployment date from oldest data directory
	var deployTime time.Time
	if dnnDataErr == nil && dnnDataStat != nil {
		deployTime = dnnDataStat.ModTime()
	}
	if bvnnDataErr == nil && bvnnDataStat != nil {
		if deployTime.IsZero() || bvnnDataStat.ModTime().Before(deployTime) {
			deployTime = bvnnDataStat.ModTime()
		}
	}
	// Also check dnn directory itself
	if dnnStat, err := os.Stat(dnnDir); err == nil {
		if deployTime.IsZero() || dnnStat.ModTime().Before(deployTime) {
			deployTime = dnnStat.ModTime()
		}
	}
	if !deployTime.IsZero() {
		m.deployment.DeployedAt = deployTime
		m.deployment.DeployedAge = formatUptime(time.Since(deployTime))
	}

	// Check binary version and update availability
	runningBinary := filepath.Join(*workDir, "accumulated")
	if stat, err := os.Stat(runningBinary); err == nil {
		m.deployment.BinaryAge = formatUptime(time.Since(stat.ModTime()))

		// Check if artifacts directory has newer binary
		if *artifactsDir != "" {
			artifactBinary := filepath.Join(*artifactsDir, "binaries", "accumulated")
			if artifactStat, err := os.Stat(artifactBinary); err == nil {
				if artifactStat.ModTime().After(stat.ModTime()) {
					m.deployment.UpdateAvailable = true
				} else {
					m.deployment.UpdateAvailable = false
				}
			}
		}
	}

	// Check if follower is running - first check PID file, then check if RPC responds
	pid := m.getPID("follower.pid")
	pidRunning := pid > 0 && m.isProcessRunning(pid)

	// Also check if follower is responding on RPC (it might be running without PID file)
	followerResponding := m.follower.Available

	if pidRunning {
		m.deployment.State = StateRunning
		m.deployment.PID = pid

		// Get process start time for uptime
		if procStat, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
			m.deployment.StartedAt = procStat.ModTime()
			m.deployment.Uptime = formatUptime(time.Since(procStat.ModTime()))
		}
	} else if followerResponding {
		// Follower is running but we don't have PID file - find the process
		m.deployment.State = StateRunning
		m.deployment.PID = m.findFollowerPID()
		if m.deployment.PID > 0 {
			if procStat, err := os.Stat(fmt.Sprintf("/proc/%d", m.deployment.PID)); err == nil {
				m.deployment.StartedAt = procStat.ModTime()
				m.deployment.Uptime = formatUptime(time.Since(procStat.ModTime()))
			}
		}
	} else if !m.deployment.DNInitialized && !m.deployment.BVNInitialized {
		m.deployment.State = StateNotDeployed
		m.deployment.PID = 0
		m.deployment.Uptime = ""
	} else {
		m.deployment.State = StateStopped
		m.deployment.PID = 0
		m.deployment.Uptime = ""
	}

	// Read recent logs
	m.updateRecentLogs()
}

func (m *Monitor) findFollowerPID() int {
	// Try to find accumulated process running run-dual
	cmd := exec.Command("pgrep", "-f", "accumulated.*run-dual")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 && lines[0] != "" {
		pid, _ := strconv.Atoi(lines[0])
		return pid
	}
	return 0
}

func (m *Monitor) updateRecentLogs() {
	if *workDir == "" {
		return
	}

	logsDir := filepath.Join(*workDir, "logs")
	logFiles := []struct {
		path   string
		source string
	}{
		{filepath.Join(logsDir, "follower.log"), "follower"},
		{filepath.Join(logsDir, "init-dn.log"), "init-dn"},
		{filepath.Join(logsDir, "init-bvn.log"), "init-bvn"},
	}

	var allLogs []LogEntry

	for _, lf := range logFiles {
		logs := m.tailLogFile(lf.path, lf.source, 20)
		allLogs = append(allLogs, logs...)
	}

	// Sort by time (most recent first) and limit
	if len(allLogs) > maxLogLines {
		allLogs = allLogs[len(allLogs)-maxLogLines:]
	}

	m.deployment.RecentLogs = allLogs
}

func (m *Monitor) tailLogFile(path, source string, lines int) []LogEntry {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return nil
	}

	// Read last N lines
	var entries []LogEntry

	// Simple approach: read from end
	size := stat.Size()
	if size == 0 {
		return nil
	}

	// Read last 64KB or whole file if smaller
	readSize := int64(65536)
	if size < readSize {
		readSize = size
	}

	offset := size - readSize
	if offset < 0 {
		offset = 0
	}

	file.Seek(offset, 0)
	scanner := bufio.NewScanner(file)

	var lineBuffer []string
	for scanner.Scan() {
		lineBuffer = append(lineBuffer, scanner.Text())
	}

	// Take last N lines
	start := 0
	if len(lineBuffer) > lines {
		start = len(lineBuffer) - lines
	}

	for i := start; i < len(lineBuffer); i++ {
		line := lineBuffer[i]
		if line == "" {
			continue
		}

		entry := LogEntry{
			Time:    time.Now(),
			Source:  source,
			Message: line,
			Level:   detectLogLevel(line),
		}
		entries = append(entries, entry)
	}

	return entries
}

func detectLogLevel(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "err=") || strings.Contains(lower, "failed") {
		return "error"
	}
	if strings.Contains(lower, "warn") {
		return "warning"
	}
	if strings.Contains(lower, "info") || strings.Contains(lower, "module=") {
		return "info"
	}
	return "debug"
}

func (m *Monitor) getPID(pidFile string) int {
	if *workDir == "" {
		return 0
	}
	path := filepath.Join(*workDir, pidFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

func (m *Monitor) isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func (m *Monitor) calculateRates() {
	now := time.Now()
	elapsed := now.Sub(m.lastCheckTime).Minutes()

	if elapsed > 0 && m.lastCheckTime.Unix() > 0 {
		m.mainnet.DNBlockRate = float64(m.mainnet.DNHeight-m.lastMainnetDN) / elapsed
		m.mainnet.BVNBlockRate = float64(m.mainnet.BVNHeight-m.lastMainnetBVN) / elapsed
		m.follower.DNBlockRate = float64(m.follower.DNHeight-m.lastFollowerDN) / elapsed
		m.follower.BVNBlockRate = float64(m.follower.BVNHeight-m.lastFollowerBVN) / elapsed
	}

	if m.followerLaunchDNHeight == 0 {
		m.followerLaunchDNHeight = m.follower.DNHeight
		m.followerLaunchBVNHeight = m.follower.BVNHeight
		m.oneHourDNHeight = m.follower.DNHeight
		m.oneHourBVNHeight = m.follower.BVNHeight
		m.twoHourDNHeight = m.follower.DNHeight
		m.twoHourBVNHeight = m.follower.BVNHeight
	}

	sinceLaunch := now.Sub(m.followerLaunchTime).Minutes()
	if sinceLaunch > 0 {
		m.follower.DNSinceLaunch = float64(m.follower.DNHeight-m.followerLaunchDNHeight) / sinceLaunch
		m.follower.BVNSinceLaunch = float64(m.follower.BVNHeight-m.followerLaunchBVNHeight) / sinceLaunch
	}

	hoursSinceLaunch := now.Sub(m.followerLaunchTime).Hours()
	if hoursSinceLaunch < 1 {
		m.follower.DNRollingRate = m.follower.DNSinceLaunch
		m.follower.BVNRollingRate = m.follower.BVNSinceLaunch
	} else if hoursSinceLaunch < 2 {
		oneHourElapsed := now.Sub(m.oneHourStartTime).Minutes()
		if oneHourElapsed >= 60 {
			m.oneHourStartTime = now
			m.oneHourDNHeight = m.follower.DNHeight
			m.oneHourBVNHeight = m.follower.BVNHeight
		}
		oneHourMinutes := now.Sub(m.oneHourStartTime).Minutes()
		if oneHourMinutes > 0 {
			m.follower.DNRollingRate = float64(m.follower.DNHeight-m.oneHourDNHeight) / oneHourMinutes
			m.follower.BVNRollingRate = float64(m.follower.BVNHeight-m.oneHourBVNHeight) / oneHourMinutes
		}
	} else {
		twoHourElapsed := now.Sub(m.twoHourStartTime).Minutes()
		if twoHourElapsed >= 120 {
			m.twoHourStartTime = now
			m.twoHourDNHeight = m.follower.DNHeight
			m.twoHourBVNHeight = m.follower.BVNHeight
		}
		twoHourMinutes := now.Sub(m.twoHourStartTime).Minutes()
		if twoHourMinutes > 0 {
			m.follower.DNRollingRate = float64(m.follower.DNHeight-m.twoHourDNHeight) / twoHourMinutes
			m.follower.BVNRollingRate = float64(m.follower.BVNHeight-m.twoHourBVNHeight) / twoHourMinutes
		}
	}

	m.calculateMainnetHistoricalRates()

	m.lastMainnetDN = m.mainnet.DNHeight
	m.lastMainnetBVN = m.mainnet.BVNHeight
	m.lastFollowerDN = m.follower.DNHeight
	m.lastFollowerBVN = m.follower.BVNHeight
	m.lastCheckTime = now
}

func (m *Monitor) calculateMainnetHistoricalRates() {
	if m.mainnet.DNHeight > 2 {
		genesisBlock, err := fetchBlock(mainnetDNRPC, 2)
		if err == nil && genesisBlock.Result.Block.Header.Time != "" {
			genesisTime, err := parseTime(genesisBlock.Result.Block.Header.Time)
			if err == nil {
				currentBlock, err := fetchBlock(mainnetDNRPC, m.mainnet.DNHeight)
				if err == nil && currentBlock.Result.Block.Header.Time != "" {
					currentTime, err := parseTime(currentBlock.Result.Block.Header.Time)
					if err == nil {
						m.mainnet.DNGenesisRate = calculateBlockRate(m.mainnet.DNHeight, currentTime, 2, genesisTime)
					}
				}
			}
		}
	}

	if m.mainnet.BVNHeight > 2 {
		genesisBlock, err := fetchBlock(mainnetBVNRPC, 2)
		if err == nil && genesisBlock.Result.Block.Header.Time != "" {
			genesisTime, err := parseTime(genesisBlock.Result.Block.Header.Time)
			if err == nil {
				currentBlock, err := fetchBlock(mainnetBVNRPC, m.mainnet.BVNHeight)
				if err == nil && currentBlock.Result.Block.Header.Time != "" {
					currentTime, err := parseTime(currentBlock.Result.Block.Header.Time)
					if err == nil {
						m.mainnet.BVNGenesisRate = calculateBlockRate(m.mainnet.BVNHeight, currentTime, 2, genesisTime)
					}
				}
			}
		}
	}

	estimatedBlocksInWeek := int64(60 * 60 * 24 * 7)
	weekOldHeight := m.mainnet.DNHeight - estimatedBlocksInWeek
	if weekOldHeight < 2 {
		weekOldHeight = 2
	}

	if weekOldHeight > 0 {
		weekOldBlock, err := fetchBlock(mainnetDNRPC, weekOldHeight)
		if err == nil && weekOldBlock.Result.Block.Header.Time != "" {
			weekOldTime, err := parseTime(weekOldBlock.Result.Block.Header.Time)
			if err == nil {
				currentBlock, err := fetchBlock(mainnetDNRPC, m.mainnet.DNHeight)
				if err == nil && currentBlock.Result.Block.Header.Time != "" {
					currentTime, err := parseTime(currentBlock.Result.Block.Header.Time)
					if err == nil {
						m.mainnet.DNWeeklyRate = calculateBlockRate(m.mainnet.DNHeight, currentTime, weekOldHeight, weekOldTime)
					}
				}
			}
		}
	}

	weekOldHeight = m.mainnet.BVNHeight - estimatedBlocksInWeek
	if weekOldHeight < 2 {
		weekOldHeight = 2
	}

	if weekOldHeight > 0 {
		weekOldBlock, err := fetchBlock(mainnetBVNRPC, weekOldHeight)
		if err == nil && weekOldBlock.Result.Block.Header.Time != "" {
			weekOldTime, err := parseTime(weekOldBlock.Result.Block.Header.Time)
			if err == nil {
				currentBlock, err := fetchBlock(mainnetBVNRPC, m.mainnet.BVNHeight)
				if err == nil && currentBlock.Result.Block.Header.Time != "" {
					currentTime, err := parseTime(currentBlock.Result.Block.Header.Time)
					if err == nil {
						m.mainnet.BVNWeeklyRate = calculateBlockRate(m.mainnet.BVNHeight, currentTime, weekOldHeight, weekOldTime)
					}
				}
			}
		}
	}
}

func (m *Monitor) detectAlerts() {
	alerts := []Alert{}
	now := time.Now()

	if !m.follower.Available {
		alerts = append(alerts, Alert{
			Level:   "critical",
			Message: fmt.Sprintf("Follower is unavailable: %s", m.follower.Error),
			Time:    now,
		})
	}

	if !m.mainnet.Available {
		alerts = append(alerts, Alert{
			Level:   "critical",
			Message: "Mainnet is unavailable",
			Time:    now,
		})
	}

	if m.follower.Available && m.mainnet.Available {
		if m.comparison.DNLag > 10000 {
			alerts = append(alerts, Alert{
				Level:   "critical",
				Message: fmt.Sprintf("DN sync lag is very high: %d blocks behind", m.comparison.DNLag),
				Time:    now,
			})
		} else if m.comparison.DNLag > 1000 {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Message: fmt.Sprintf("DN sync lag is high: %d blocks behind", m.comparison.DNLag),
				Time:    now,
			})
		}

		if m.comparison.BVNLag > 10000 {
			alerts = append(alerts, Alert{
				Level:   "critical",
				Message: fmt.Sprintf("BVN sync lag is very high: %d blocks behind", m.comparison.BVNLag),
				Time:    now,
			})
		} else if m.comparison.BVNLag > 1000 {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Message: fmt.Sprintf("BVN sync lag is high: %d blocks behind", m.comparison.BVNLag),
				Time:    now,
			})
		}

		if m.follower.DNPeers == 0 {
			alerts = append(alerts, Alert{
				Level:   "critical",
				Message: "Follower DN has no peer connections",
				Time:    now,
			})
		}

		if m.follower.BVNPeers == 0 {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Message: "Follower BVN has no peer connections",
				Time:    now,
			})
		}

		if m.follower.DNCatchingUp && m.follower.DNBlockRate < 100 {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Message: fmt.Sprintf("DN sync rate is slow: %.0f blocks/min", m.follower.DNBlockRate),
				Time:    now,
			})
		}

		if m.follower.BVNCatchingUp && m.follower.BVNBlockRate < 100 {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Message: fmt.Sprintf("BVN sync rate is slow: %.0f blocks/min", m.follower.BVNBlockRate),
				Time:    now,
			})
		}

		if !m.follower.DNCatchingUp && !m.follower.BVNCatchingUp {
			alerts = append(alerts, Alert{
				Level:   "info",
				Message: "Follower is fully synced with mainnet",
				Time:    now,
			})
		}
	}

	m.comparison.Alerts = alerts
}

func (m *Monitor) Update() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fetchNodeMetrics(m.mainnet, mainnetDNRPC, mainnetBVNRPC)
	m.fetchNodeMetrics(m.follower, followerDNRPC, followerBVNRPC)
	m.calculateRates()
	m.updateDeploymentStatus()

	if m.mainnet.Available && m.follower.Available {
		m.comparison.DNLag = m.mainnet.DNHeight - m.follower.DNHeight
		m.comparison.BVNLag = m.mainnet.BVNHeight - m.follower.BVNHeight

		if m.mainnet.DNHeight > 0 {
			m.comparison.DNSyncPercent = (float64(m.follower.DNHeight) / float64(m.mainnet.DNHeight)) * 100
		}
		if m.mainnet.BVNHeight > 0 {
			m.comparison.BVNSyncPercent = (float64(m.follower.BVNHeight) / float64(m.mainnet.BVNHeight)) * 100
		}

		if m.follower.DNBlockRate > 0 && m.comparison.DNLag > 0 {
			minutesRemaining := float64(m.comparison.DNLag) / m.follower.DNBlockRate
			m.comparison.DNETA = formatDuration(minutesRemaining)
		} else {
			m.comparison.DNETA = "Calculating..."
		}

		if m.follower.BVNBlockRate > 0 && m.comparison.BVNLag > 0 {
			minutesRemaining := float64(m.comparison.BVNLag) / m.follower.BVNBlockRate
			m.comparison.BVNETA = formatDuration(minutesRemaining)
		} else {
			m.comparison.BVNETA = "Calculating..."
		}
	}

	m.comparison.LastCheck = time.Now()
	m.detectAlerts()
}

func (m *Monitor) GetSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"mainnet":    m.mainnet,
		"follower":   m.follower,
		"comparison": m.comparison,
		"deployment": m.deployment,
		"ports":      m.ports,
	}
}

func (m *Monitor) StartFollower() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if *workDir == "" {
		return fmt.Errorf("work directory not configured")
	}

	// Check if already running
	pid := m.getPID("follower.pid")
	if pid > 0 && m.isProcessRunning(pid) {
		return fmt.Errorf("follower is already running (PID %d)", pid)
	}

	// Check for start script
	startScript := filepath.Join(*workDir, "start.sh")
	if _, err := os.Stat(startScript); err != nil {
		return fmt.Errorf("start.sh not found - deploy first")
	}

	m.deployment.State = StateStarting

	// Execute start script
	cmd := exec.Command("/bin/bash", startScript)
	cmd.Dir = *workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.deployment.State = StateStopped
		return fmt.Errorf("failed to start: %v\nOutput: %s", err, string(output))
	}

	// Wait a moment for process to start
	time.Sleep(2 * time.Second)

	pid = m.getPID("follower.pid")
	if pid > 0 && m.isProcessRunning(pid) {
		m.deployment.State = StateRunning
		m.deployment.PID = pid
		return nil
	}

	m.deployment.State = StateStopped
	return fmt.Errorf("follower started but process not found")
}

func (m *Monitor) StopFollower() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.stopFollowerInternal()
}

func (m *Monitor) stopFollowerInternal() error {
	if *workDir == "" {
		return fmt.Errorf("work directory not configured")
	}

	pid := m.getPID("follower.pid")
	if pid == 0 {
		// Try to find by process
		pid = m.findFollowerPID()
	}

	if pid == 0 {
		m.deployment.State = StateStopped
		return nil
	}

	if !m.isProcessRunning(pid) {
		// Clean up stale PID file
		os.Remove(filepath.Join(*workDir, "follower.pid"))
		m.deployment.State = StateStopped
		return nil
	}

	m.deployment.State = StateStopping

	// Send SIGTERM
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %v", err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %v", err)
	}

	// Wait for graceful shutdown (up to 30 seconds)
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		if !m.isProcessRunning(pid) {
			os.Remove(filepath.Join(*workDir, "follower.pid"))
			m.deployment.State = StateStopped
			m.deployment.PID = 0
			return nil
		}
	}

	// Force kill if still running
	proc.Signal(syscall.SIGKILL)
	time.Sleep(1 * time.Second)
	os.Remove(filepath.Join(*workDir, "follower.pid"))
	m.deployment.State = StateStopped
	m.deployment.PID = 0

	return nil
}

func (m *Monitor) UpdateBinaries() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if *workDir == "" {
		return fmt.Errorf("work directory not configured")
	}
	if *artifactsDir == "" {
		return fmt.Errorf("artifacts directory not configured")
	}

	wasRunning := m.deployment.State == StateRunning

	// Stop if running
	if wasRunning {
		if err := m.stopFollowerInternal(); err != nil {
			return fmt.Errorf("failed to stop follower: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	// Copy new binaries
	binaries := []string{"accumulated", "follower-monitor"}
	for _, bin := range binaries {
		src := filepath.Join(*artifactsDir, "binaries", bin)
		dst := filepath.Join(*workDir, bin)

		if _, err := os.Stat(src); err != nil {
			continue // Skip if source doesn't exist
		}

		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", bin, err)
		}
		if err := os.WriteFile(dst, data, 0755); err != nil {
			return fmt.Errorf("failed to write %s: %v", bin, err)
		}
	}

	m.deployment.UpdateAvailable = false

	// Restart if it was running
	if wasRunning {
		startScript := filepath.Join(*workDir, "start.sh")
		if _, err := os.Stat(startScript); err == nil {
			cmd := exec.Command("/bin/bash", startScript)
			cmd.Dir = *workDir
			if _, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to restart: %v", err)
			}
			time.Sleep(2 * time.Second)
			m.deployment.State = StateRunning
		}
	}

	return nil
}

func (m *Monitor) GetLogs(source string, lines int) []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if source == "" {
		return m.deployment.RecentLogs
	}

	var filtered []LogEntry
	for _, log := range m.deployment.RecentLogs {
		if log.Source == source {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

func (m *Monitor) StreamLogs(source string) ([]byte, error) {
	if *workDir == "" {
		return nil, fmt.Errorf("work directory not configured")
	}

	var logPath string
	switch source {
	case "follower":
		logPath = filepath.Join(*workDir, "logs", "follower.log")
	case "init-dn":
		logPath = filepath.Join(*workDir, "logs", "init-dn.log")
	case "init-bvn":
		logPath = filepath.Join(*workDir, "logs", "init-bvn.log")
	default:
		logPath = filepath.Join(*workDir, "logs", "follower.log")
	}

	// Read last 50KB of log file
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	readSize := int64(51200)
	if size < readSize {
		readSize = size
	}

	offset := size - readSize
	if offset < 0 {
		offset = 0
	}

	file.Seek(offset, 0)
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return data, nil
}

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

func fetchStatus(rpcURL string) (*CometStatus, error) {
	resp, err := httpClient.Get(rpcURL + "/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status CometStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return &status, nil
}

func fetchNetInfo(rpcURL string) (*NetInfo, error) {
	resp, err := httpClient.Get(rpcURL + "/net_info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var netInfo NetInfo
	if err := json.NewDecoder(resp.Body).Decode(&netInfo); err != nil {
		return nil, err
	}

	return &netInfo, nil
}

func parseInt64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func fetchBlock(rpcURL string, height int64) (*BlockInfo, error) {
	url := fmt.Sprintf("%s/block?height=%d", rpcURL, height)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var blockInfo BlockInfo
	if err := json.NewDecoder(resp.Body).Decode(&blockInfo); err != nil {
		return nil, err
	}

	return &blockInfo, nil
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func calculateBlockRate(currentHeight int64, currentTime time.Time, oldHeight int64, oldTime time.Time) float64 {
	if currentHeight <= oldHeight || currentTime.Before(oldTime) {
		return 0
	}
	blocksDiff := float64(currentHeight - oldHeight)
	timeDiff := currentTime.Sub(oldTime).Minutes()
	if timeDiff == 0 {
		return 0
	}
	return blocksDiff / timeDiff
}

func formatDuration(minutes float64) string {
	if minutes < 60 {
		return fmt.Sprintf("%.0f min", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%.1f hours", hours)
	}
	days := hours / 24
	if days < 7 {
		return fmt.Sprintf("%.1f days", days)
	}
	weeks := days / 7
	return fmt.Sprintf("%.1f weeks", weeks)
}

func main() {
	flag.Parse()

	monitor := NewMonitor()

	if *workDir != "" {
		log.Printf("Monitoring work directory: %s", *workDir)
	} else {
		log.Println("Warning: No work directory specified (--work-dir). Deployment management disabled.")
	}

	// Initial update
	monitor.Update()

	// Start update loop
	go func() {
		ticker := time.NewTicker(time.Duration(*interval) * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			monitor.Update()
		}
	}()

	// HTTP handlers
	http.HandleFunc("/", handleDashboard)

	http.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(monitor.GetSnapshot())
	})

	http.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(monitor.GetLogs(source, 100))
	})

	http.HandleFunc("/api/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		data, err := monitor.StreamLogs(source)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write(data)
	})

	http.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := monitor.StartFollower(); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Follower started",
		})
	})

	http.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := monitor.StopFollower(); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Follower stopped",
		})
	})

	http.HandleFunc("/api/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := monitor.UpdateBinaries(); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Binaries updated successfully",
		})
	})

	addr := fmt.Sprintf("%s:%d", *bindAddr, *port)
	log.Printf("Follower Monitor starting on http://%s", addr)
	if *bindAddr == "127.0.0.1" {
		log.Println("  (bound to localhost only - use --bind 0.0.0.0 to expose externally)")
	}
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	tmpl.Execute(w, nil)
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Accumulate Follower Monitor</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: #1a1a1a;
            color: #e0e0e0;
            padding: 10px;
            font-size: 13px;
        }
        .container { max-width: 1800px; margin: 0 auto; }
        header {
            background: #2a2a2a;
            padding: 8px 15px;
            border-radius: 4px;
            margin-bottom: 10px;
            border-left: 3px solid #4a9eff;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        h1 { font-size: 18px; font-weight: 600; color: #fff; }
        .header-controls { display: flex; gap: 10px; align-items: center; }
        .tabs {
            display: flex;
            gap: 5px;
            margin-bottom: 10px;
        }
        .tab {
            padding: 8px 16px;
            background: #2a2a2a;
            border: none;
            border-radius: 4px 4px 0 0;
            color: #999;
            cursor: pointer;
            font-size: 13px;
        }
        .tab.active {
            background: #3a3a3a;
            color: #fff;
            border-bottom: 2px solid #4a9eff;
        }
        .tab:hover { color: #fff; }
        .tab-content { display: none; }
        .tab-content.active { display: block; }
        .btn {
            padding: 6px 16px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 12px;
            font-weight: 600;
            transition: all 0.2s;
        }
        .btn-start {
            background: #1a4a1a;
            color: #4aff4a;
        }
        .btn-start:hover { background: #2a5a2a; }
        .btn-stop {
            background: #4a1a1a;
            color: #ff4a4a;
        }
        .btn-stop:hover { background: #5a2a2a; }
        .btn-update {
            background: #4a4a1a;
            color: #ffff4a;
            font-size: 10px;
            padding: 3px 8px;
        }
        .btn-update:hover { background: #5a5a2a; }
        .btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }
        .port-info {
            font-size: 10px;
            color: #666;
            font-weight: normal;
            margin-left: 8px;
        }
        .alerts {
            background: #2a2a2a;
            border-radius: 4px;
            padding: 8px;
            margin-bottom: 10px;
            display: none;
        }
        .alerts.active { display: block; }
        .alert { padding: 6px 10px; margin: 4px 0; border-radius: 3px; font-size: 12px; }
        .alert.critical { background: #4a1a1a; border-left: 3px solid #ff4444; }
        .alert.warning { background: #4a3a1a; border-left: 3px solid #ffaa44; }
        .alert.info { background: #1a3a4a; border-left: 3px solid #44aaff; }
        .grid { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 10px; }
        .grid-4 { display: grid; grid-template-columns: 1fr 1fr 1fr 1fr; gap: 10px; }
        .card {
            background: #2a2a2a;
            border-radius: 4px;
            padding: 10px;
            border-left: 3px solid #555;
        }
        .card.deployment { border-left-color: #9b59b6; }
        .card-title {
            font-size: 14px;
            font-weight: 600;
            margin-bottom: 8px;
            color: #fff;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .status { padding: 2px 8px; border-radius: 3px; font-size: 11px; font-weight: 600; }
        .status.online { background: #1a4a1a; color: #4aff4a; }
        .status.syncing { background: #2a3a4a; color: #4a9eff; }
        .status.offline { background: #4a1a1a; color: #ff4444; }
        .status.stopped { background: #3a3a3a; color: #999; }
        .status.starting { background: #4a4a1a; color: #ffff4a; }
        table { width: 100%; border-collapse: collapse; }
        td { padding: 4px 0; border-bottom: 1px solid #333; }
        td:first-child { color: #999; width: 45%; }
        td:last-child { text-align: right; font-weight: 600; color: #fff; }
        .value-large { font-size: 20px; color: #4a9eff; }
        .progress-container { margin: 6px 0; }
        .progress-label { display: flex; justify-content: space-between; margin-bottom: 3px; font-size: 11px; }
        .progress-bar {
            width: 100%;
            height: 20px;
            background: #1a1a1a;
            border-radius: 3px;
            overflow: hidden;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #4a9eff 0%, #357abd 100%);
            transition: width 0.5s ease;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #fff;
            font-size: 11px;
            font-weight: 600;
        }
        .footer {
            text-align: center;
            margin-top: 10px;
            color: #666;
            font-size: 11px;
        }
        .section { margin: 8px 0; }
        .section-title {
            font-size: 12px;
            font-weight: 600;
            color: #4a9eff;
            margin-bottom: 4px;
            border-bottom: 1px solid #333;
            padding-bottom: 2px;
        }
        .log-container {
            background: #1a1a1a;
            border-radius: 4px;
            padding: 10px;
            height: 400px;
            overflow-y: auto;
            font-family: monospace;
            font-size: 11px;
        }
        .log-entry {
            padding: 2px 0;
            border-bottom: 1px solid #222;
        }
        .log-entry.error { color: #ff6b6b; }
        .log-entry.warning { color: #ffa726; }
        .log-entry.info { color: #4a9eff; }
        .log-entry.debug { color: #888; }
        .log-source {
            display: inline-block;
            padding: 1px 6px;
            border-radius: 3px;
            font-size: 10px;
            margin-right: 8px;
        }
        .log-source.follower { background: #2a4a2a; color: #4aff4a; }
        .log-source.init-dn { background: #2a3a4a; color: #4a9eff; }
        .log-source.init-bvn { background: #4a3a2a; color: #ffaa44; }
        .log-controls {
            display: flex;
            gap: 10px;
            margin-bottom: 10px;
        }
        .log-filter {
            padding: 4px 8px;
            background: #3a3a3a;
            border: none;
            border-radius: 3px;
            color: #fff;
            font-size: 12px;
            cursor: pointer;
        }
        .log-filter.active { background: #4a9eff; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Accumulate Follower Monitor</h1>
            <div class="header-controls">
                <span id="deployment-state" class="status stopped">Unknown</span>
                <button id="btn-start" class="btn btn-start" onclick="startFollower()">Start</button>
                <button id="btn-stop" class="btn btn-stop" onclick="stopFollower()">Stop</button>
            </div>
        </header>

        <div class="tabs">
            <button class="tab active" onclick="showTab('status')">Status</button>
            <button class="tab" onclick="showTab('logs')">Logs</button>
            <button class="tab" onclick="showTab('deployment')">Deployment</button>
        </div>

        <div id="tab-status" class="tab-content active">
            <div id="alerts" class="alerts"></div>

            <div class="grid-4">
                <div class="card deployment">
                    <div class="card-title">
                        <span>Deployment</span>
                        <button id="btn-update" class="btn btn-update" onclick="updateBinaries()" style="display:none;">Update</button>
                    </div>
                    <table>
                        <tr><td>State</td><td id="deploy-state">-</td></tr>
                        <tr><td>Deployed</td><td id="deploy-date">-</td></tr>
                        <tr><td>Binary Age</td><td id="deploy-binary-age">-</td></tr>
                        <tr><td>PID</td><td id="deploy-pid">-</td></tr>
                        <tr><td>Uptime</td><td id="deploy-uptime">-</td></tr>
                    </table>
                </div>

                <div class="card">
                    <div class="card-title">
                        <span>MainNet (Apollo)</span>
                        <span id="mainnet-status" class="status">...</span>
                    </div>
                    <div class="section">
                        <div class="section-title">Directory Network</div>
                        <table>
                            <tr><td>Height</td><td id="mainnet-dn-height" class="value-large">-</td></tr>
                            <tr><td>Peers</td><td id="mainnet-dn-peers">-</td></tr>
                            <tr><td>Current</td><td id="mainnet-dn-rate">-</td></tr>
                        </table>
                    </div>
                    <div class="section">
                        <div class="section-title">BVN Cyclops</div>
                        <table>
                            <tr><td>Height</td><td id="mainnet-bvn-height" class="value-large">-</td></tr>
                            <tr><td>Peers</td><td id="mainnet-bvn-peers">-</td></tr>
                            <tr><td>Current</td><td id="mainnet-bvn-rate">-</td></tr>
                        </table>
                    </div>
                </div>

                <div class="card">
                    <div class="card-title">
                        <span>Local Follower</span>
                        <span id="follower-status" class="status">...</span>
                    </div>
                    <div class="section">
                        <div class="section-title">Directory Network <span class="port-info" id="dn-ports"></span></div>
                        <table>
                            <tr><td>Height</td><td id="follower-dn-height" class="value-large">-</td></tr>
                            <tr><td>Peers</td><td id="follower-dn-peers">-</td></tr>
                            <tr><td>Current</td><td id="follower-dn-rate">-</td></tr>
                            <tr><td>Rolling</td><td id="follower-dn-rolling">-</td></tr>
                        </table>
                    </div>
                    <div class="section">
                        <div class="section-title">BVN Cyclops <span class="port-info" id="bvn-ports"></span></div>
                        <table>
                            <tr><td>Height</td><td id="follower-bvn-height" class="value-large">-</td></tr>
                            <tr><td>Peers</td><td id="follower-bvn-peers">-</td></tr>
                            <tr><td>Current</td><td id="follower-bvn-rate">-</td></tr>
                            <tr><td>Rolling</td><td id="follower-bvn-rolling">-</td></tr>
                        </table>
                    </div>
                </div>

                <div class="card">
                    <div class="card-title">Sync Status</div>
                    <div class="section">
                        <div class="section-title">Directory Network</div>
                        <div class="progress-container">
                            <div class="progress-label">
                                <span id="dn-sync-text">0%</span>
                                <span id="dn-eta">-</span>
                            </div>
                            <div class="progress-bar">
                                <div id="dn-progress" class="progress-fill" style="width: 0%">0%</div>
                            </div>
                        </div>
                        <table>
                            <tr><td>Behind</td><td id="dn-lag">-</td></tr>
                        </table>
                    </div>
                    <div class="section">
                        <div class="section-title">BVN Cyclops</div>
                        <div class="progress-container">
                            <div class="progress-label">
                                <span id="bvn-sync-text">0%</span>
                                <span id="bvn-eta">-</span>
                            </div>
                            <div class="progress-bar">
                                <div id="bvn-progress" class="progress-fill" style="width: 0%">0%</div>
                            </div>
                        </div>
                        <table>
                            <tr><td>Behind</td><td id="bvn-lag">-</td></tr>
                        </table>
                    </div>
                </div>
            </div>
        </div>

        <div id="tab-logs" class="tab-content">
            <div class="log-controls">
                <button class="log-filter active" onclick="filterLogs('')" data-filter="">All</button>
                <button class="log-filter" onclick="filterLogs('follower')" data-filter="follower">Follower</button>
                <button class="log-filter" onclick="filterLogs('init-dn')" data-filter="init-dn">Init DN</button>
                <button class="log-filter" onclick="filterLogs('init-bvn')" data-filter="init-bvn">Init BVN</button>
                <button class="log-filter" onclick="filterLogs('error')" data-filter="error">Errors</button>
                <span style="margin-left: auto; color: #666;">Auto-refresh every 5s</span>
            </div>
            <div id="log-container" class="log-container"></div>
        </div>

        <div id="tab-deployment" class="tab-content">
            <div class="card">
                <div class="card-title">Deployment Information</div>
                <table>
                    <tr><td>Work Directory</td><td id="deploy-info-workdir">-</td></tr>
                    <tr><td>DN Initialized</td><td id="deploy-info-dn">-</td></tr>
                    <tr><td>BVN Initialized</td><td id="deploy-info-bvn">-</td></tr>
                    <tr><td>Config Exists</td><td id="deploy-info-config">-</td></tr>
                    <tr><td>Current State</td><td id="deploy-info-state">-</td></tr>
                    <tr><td>Process ID</td><td id="deploy-info-pid">-</td></tr>
                    <tr><td>Uptime</td><td id="deploy-info-uptime">-</td></tr>
                </table>
            </div>
        </div>

        <div class="footer">
            Last updated: <span id="last-update">Never</span>
        </div>
    </div>

    <script>
        let currentLogFilter = '';

        function fmt(num) {
            return new Intl.NumberFormat().format(num);
        }

        function fmtRate(rate) {
            if (rate === 0) return "0 bl/min";
            return Math.round(rate) + " bl/min";
        }

        function showTab(name) {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
            document.querySelector('.tab[onclick="showTab(\'' + name + '\')"]').classList.add('active');
            document.getElementById('tab-' + name).classList.add('active');

            if (name === 'logs') {
                fetchLogs();
            }
        }

        function filterLogs(filter) {
            currentLogFilter = filter;
            document.querySelectorAll('.log-filter').forEach(b => b.classList.remove('active'));
            document.querySelector('.log-filter[data-filter="' + filter + '"]').classList.add('active');
            fetchLogs();
        }

        function updateUI(data) {
            const { mainnet, follower, comparison, deployment } = data;

            // Update deployment status
            if (deployment) {
                const stateMap = {
                    'unknown': { text: 'Unknown', class: 'stopped' },
                    'not_deployed': { text: 'Not Deployed', class: 'stopped' },
                    'stopped': { text: 'Stopped', class: 'stopped' },
                    'starting': { text: 'Starting...', class: 'starting' },
                    'running': { text: 'Running', class: 'online' },
                    'stopping': { text: 'Stopping...', class: 'starting' },
                    'initializing_dn': { text: 'Init DN', class: 'syncing' },
                    'initializing_bvn': { text: 'Init BVN', class: 'syncing' }
                };
                const state = stateMap[deployment.state] || stateMap['unknown'];

                document.getElementById('deployment-state').textContent = state.text;
                document.getElementById('deployment-state').className = 'status ' + state.class;
                document.getElementById('deploy-state').textContent = state.text;
                document.getElementById('deploy-pid').textContent = deployment.pid || '-';
                document.getElementById('deploy-uptime').textContent = deployment.uptime || '-';

                // Deployment date
                if (deployment.deployed_age) {
                    document.getElementById('deploy-date').textContent = deployment.deployed_age + ' ago';
                } else {
                    document.getElementById('deploy-date').textContent = '-';
                }

                // Binary age
                document.getElementById('deploy-binary-age').textContent = deployment.binary_age || '-';

                // Update button visibility
                const updateBtn = document.getElementById('btn-update');
                if (deployment.update_available) {
                    updateBtn.style.display = 'inline-block';
                } else {
                    updateBtn.style.display = 'none';
                }

                // Deployment tab
                document.getElementById('deploy-info-workdir').textContent = deployment.work_dir || '-';
                document.getElementById('deploy-info-dn').textContent = deployment.dn_initialized ? 'Yes' : 'No';
                document.getElementById('deploy-info-bvn').textContent = deployment.bvn_initialized ? 'Yes' : 'No';
                document.getElementById('deploy-info-config').textContent = deployment.config_exists ? 'Yes' : 'No';
                document.getElementById('deploy-info-state').textContent = state.text;
                document.getElementById('deploy-info-pid').textContent = deployment.pid || '-';
                document.getElementById('deploy-info-uptime').textContent = deployment.uptime || '-';

                // Update button states
                const isRunning = deployment.state === 'running';
                const isStopped = deployment.state === 'stopped' || deployment.state === 'not_deployed';
                document.getElementById('btn-start').disabled = !isStopped;
                document.getElementById('btn-stop').disabled = !isRunning;
            }

            // Update port info
            if (data.ports) {
                document.getElementById('dn-ports').textContent = 'RPC:' + data.ports.dn_rpc + ' API:' + data.ports.dn_api;
                document.getElementById('bvn-ports').textContent = 'RPC:' + data.ports.bvn_rpc + ' API:' + data.ports.bvn_api;
            }

            if (mainnet.available) {
                document.getElementById('mainnet-status').textContent = 'Online';
                document.getElementById('mainnet-status').className = 'status online';
                document.getElementById('mainnet-dn-height').textContent = fmt(mainnet.dn_height);
                document.getElementById('mainnet-bvn-height').textContent = fmt(mainnet.bvn_height);
                document.getElementById('mainnet-dn-peers').textContent = mainnet.dn_peers;
                document.getElementById('mainnet-bvn-peers').textContent = mainnet.bvn_peers;
                document.getElementById('mainnet-dn-rate').textContent = fmtRate(mainnet.dn_block_rate);
                document.getElementById('mainnet-bvn-rate').textContent = fmtRate(mainnet.bvn_block_rate);
            } else {
                document.getElementById('mainnet-status').textContent = 'Offline';
                document.getElementById('mainnet-status').className = 'status offline';
            }

            if (follower.available) {
                const status = (follower.dn_catching_up || follower.bvn_catching_up) ? 'syncing' : 'online';
                document.getElementById('follower-status').textContent = status === 'syncing' ? 'Syncing' : 'Synced';
                document.getElementById('follower-status').className = 'status ' + status;
                document.getElementById('follower-dn-height').textContent = fmt(follower.dn_height);
                document.getElementById('follower-bvn-height').textContent = fmt(follower.bvn_height);
                document.getElementById('follower-dn-peers').textContent = follower.dn_peers;
                document.getElementById('follower-bvn-peers').textContent = follower.bvn_peers;
                document.getElementById('follower-dn-rate').textContent = fmtRate(follower.dn_block_rate);
                document.getElementById('follower-bvn-rate').textContent = fmtRate(follower.bvn_block_rate);
                document.getElementById('follower-dn-rolling').textContent = fmtRate(follower.dn_rolling_rate);
                document.getElementById('follower-bvn-rolling').textContent = fmtRate(follower.bvn_rolling_rate);
            } else {
                document.getElementById('follower-status').textContent = 'Offline';
                document.getElementById('follower-status').className = 'status offline';
            }

            if (mainnet.available && follower.available) {
                const dnPercent = Math.min(comparison.dn_sync_percent, 100);
                document.getElementById('dn-sync-text').textContent = dnPercent.toFixed(2) + '%';
                document.getElementById('dn-progress').style.width = dnPercent + '%';
                document.getElementById('dn-progress').textContent = dnPercent.toFixed(1) + '%';
                document.getElementById('dn-lag').textContent = fmt(comparison.dn_lag);
                document.getElementById('dn-eta').textContent = 'ETA: ' + comparison.dn_eta;

                const bvnPercent = Math.min(comparison.bvn_sync_percent, 100);
                document.getElementById('bvn-sync-text').textContent = bvnPercent.toFixed(2) + '%';
                document.getElementById('bvn-progress').style.width = bvnPercent + '%';
                document.getElementById('bvn-progress').textContent = bvnPercent.toFixed(1) + '%';
                document.getElementById('bvn-lag').textContent = fmt(comparison.bvn_lag);
                document.getElementById('bvn-eta').textContent = 'ETA: ' + comparison.bvn_eta;
            }

            const alertsDiv = document.getElementById('alerts');
            if (comparison.alerts && comparison.alerts.length > 0) {
                alertsDiv.className = 'alerts active';
                let html = '';
                comparison.alerts.forEach(alert => {
                    html += '<div class="alert ' + alert.level + '">' + alert.message + '</div>';
                });
                alertsDiv.innerHTML = html;
            } else {
                alertsDiv.className = 'alerts';
            }

            document.getElementById('last-update').textContent = new Date(comparison.last_check).toLocaleTimeString();
        }

        function updateLogs(logs) {
            const container = document.getElementById('log-container');
            let html = '';

            logs.forEach(log => {
                if (currentLogFilter === 'error' && log.level !== 'error') return;
                if (currentLogFilter && currentLogFilter !== 'error' && log.source !== currentLogFilter) return;

                html += '<div class="log-entry ' + log.level + '">';
                html += '<span class="log-source ' + log.source + '">' + log.source + '</span>';
                html += escapeHtml(log.message);
                html += '</div>';
            });

            container.innerHTML = html || '<div style="color: #666; text-align: center; padding: 20px;">No logs available</div>';
            container.scrollTop = container.scrollHeight;
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        async function fetchMetrics() {
            try {
                const response = await fetch('/api/metrics');
                const data = await response.json();
                updateUI(data);
            } catch (error) {
                console.error('Failed to fetch metrics:', error);
            }
        }

        async function fetchLogs() {
            try {
                const response = await fetch('/api/logs');
                const data = await response.json();
                updateLogs(data || []);
            } catch (error) {
                console.error('Failed to fetch logs:', error);
            }
        }

        async function startFollower() {
            try {
                document.getElementById('btn-start').disabled = true;
                const response = await fetch('/api/start', { method: 'POST' });
                const data = await response.json();
                if (!data.success) {
                    alert('Failed to start: ' + data.error);
                }
                fetchMetrics();
            } catch (error) {
                alert('Failed to start follower: ' + error);
            }
        }

        async function stopFollower() {
            if (!confirm('Are you sure you want to stop the follower?')) return;

            try {
                document.getElementById('btn-stop').disabled = true;
                const response = await fetch('/api/stop', { method: 'POST' });
                const data = await response.json();
                if (!data.success) {
                    alert('Failed to stop: ' + data.error);
                }
                fetchMetrics();
            } catch (error) {
                alert('Failed to stop follower: ' + error);
            }
        }

        async function updateBinaries() {
            if (!confirm('Update binaries? This will stop the follower, copy new binaries, and restart.')) return;

            try {
                document.getElementById('btn-update').disabled = true;
                const response = await fetch('/api/update', { method: 'POST' });
                const data = await response.json();
                if (!data.success) {
                    alert('Failed to update: ' + data.error);
                } else {
                    alert('Update complete: ' + data.message);
                }
                fetchMetrics();
            } catch (error) {
                alert('Failed to update: ' + error);
            } finally {
                document.getElementById('btn-update').disabled = false;
            }
        }

        fetchMetrics();
        setInterval(fetchMetrics, 10000);
        setInterval(() => {
            if (document.getElementById('tab-logs').classList.contains('active')) {
                fetchLogs();
            }
        }, 5000);
    </script>
</body>
</html>`

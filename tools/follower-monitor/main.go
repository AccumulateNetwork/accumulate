package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"
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
	DNBlockRate    float64   `json:"dn_block_rate"`      // Current rate (10-sec window)
	BVNBlockRate   float64   `json:"bvn_block_rate"`     // Current rate (10-sec window)
	DNGenesisRate  float64   `json:"dn_genesis_rate"`    // Rate since genesis
	BVNGenesisRate float64   `json:"bvn_genesis_rate"`   // Rate since genesis
	DNWeeklyRate   float64   `json:"dn_weekly_rate"`     // Rate over last week
	BVNWeeklyRate  float64   `json:"bvn_weekly_rate"`    // Rate over last week
	DNSinceLaunch  float64   `json:"dn_since_launch"`    // Follower: rate since monitor launch
	BVNSinceLaunch float64   `json:"bvn_since_launch"`   // Follower: rate since monitor launch
	DNRollingRate  float64   `json:"dn_rolling_rate"`    // Follower: 1-2 hour rolling rate
	BVNRollingRate float64   `json:"bvn_rolling_rate"`   // Follower: 1-2 hour rolling rate
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
	Level   string    `json:"level"`   // "info", "warning", "critical"
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

type Monitor struct {
	mainnet    *NodeMetrics
	follower   *NodeMetrics
	comparison *ComparisonMetrics
	mu         sync.RWMutex

	// For rate calculation
	lastMainnetDN  int64
	lastMainnetBVN int64
	lastFollowerDN  int64
	lastFollowerBVN int64
	lastCheckTime   time.Time

	// Follower launch tracking
	followerLaunchTime   time.Time
	followerLaunchDNHeight  int64
	followerLaunchBVNHeight int64

	// Rolling window tracking (1-2 hours)
	oneHourStartTime   time.Time
	oneHourDNHeight    int64
	oneHourBVNHeight   int64
	twoHourStartTime   time.Time
	twoHourDNHeight    int64
	twoHourBVNHeight   int64
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
	mainnetDNRPC  = "http://23.22.212.106:16592"
	mainnetBVNRPC = "http://23.22.212.106:16692"
	followerDNRPC  = "http://localhost:16592"
	followerBVNRPC = "http://localhost:16692"
)

func NewMonitor() *Monitor {
	now := time.Now()
	return &Monitor{
		mainnet: &NodeMetrics{Name: "apollo-mainnet"},
		follower: &NodeMetrics{Name: "follower"},
		comparison: &ComparisonMetrics{Alerts: []Alert{}},
		lastCheckTime: now,
		followerLaunchTime: now,
		oneHourStartTime: now,
		twoHourStartTime: now,
	}
}

func (m *Monitor) fetchNodeMetrics(node *NodeMetrics, dnRPC, bvnRPC string) {
	var wg sync.WaitGroup
	wg.Add(2)

	var dnErr, bvnErr error

	// Fetch DN metrics
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

	// Fetch BVN metrics
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

	// Node is available if at least one partition responds
	if dnErr != nil && bvnErr != nil {
		node.Available = false
		node.Error = fmt.Sprintf("DN: %v, BVN: %v", dnErr, bvnErr)
	} else {
		node.Available = true
		node.Error = ""
	}
}

func (m *Monitor) calculateRates() {
	now := time.Now()
	elapsed := now.Sub(m.lastCheckTime).Minutes()

	// Current rate (10-second window)
	if elapsed > 0 && m.lastCheckTime.Unix() > 0 {
		m.mainnet.DNBlockRate = float64(m.mainnet.DNHeight - m.lastMainnetDN) / elapsed
		m.mainnet.BVNBlockRate = float64(m.mainnet.BVNHeight - m.lastMainnetBVN) / elapsed
		m.follower.DNBlockRate = float64(m.follower.DNHeight - m.lastFollowerDN) / elapsed
		m.follower.BVNBlockRate = float64(m.follower.BVNHeight - m.lastFollowerBVN) / elapsed
	}

	// Initialize follower launch heights on first update
	if m.followerLaunchDNHeight == 0 {
		m.followerLaunchDNHeight = m.follower.DNHeight
		m.followerLaunchBVNHeight = m.follower.BVNHeight
		m.oneHourDNHeight = m.follower.DNHeight
		m.oneHourBVNHeight = m.follower.BVNHeight
		m.twoHourDNHeight = m.follower.DNHeight
		m.twoHourBVNHeight = m.follower.BVNHeight
	}

	// Follower: Since-launch rate
	sinceLaunch := now.Sub(m.followerLaunchTime).Minutes()
	if sinceLaunch > 0 {
		m.follower.DNSinceLaunch = float64(m.follower.DNHeight - m.followerLaunchDNHeight) / sinceLaunch
		m.follower.BVNSinceLaunch = float64(m.follower.BVNHeight - m.followerLaunchBVNHeight) / sinceLaunch
	}

	// Follower: Rolling 1-2 hour rate
	hoursSinceLaunch := now.Sub(m.followerLaunchTime).Hours()
	if hoursSinceLaunch < 1 {
		// Less than 1 hour: use since-launch rate
		m.follower.DNRollingRate = m.follower.DNSinceLaunch
		m.follower.BVNRollingRate = m.follower.BVNSinceLaunch
	} else if hoursSinceLaunch < 2 {
		// Between 1-2 hours: use 1-hour window
		oneHourElapsed := now.Sub(m.oneHourStartTime).Minutes()
		if oneHourElapsed >= 60 {
			// Reset 1-hour window
			m.oneHourStartTime = now
			m.oneHourDNHeight = m.follower.DNHeight
			m.oneHourBVNHeight = m.follower.BVNHeight
		}
		oneHourMinutes := now.Sub(m.oneHourStartTime).Minutes()
		if oneHourMinutes > 0 {
			m.follower.DNRollingRate = float64(m.follower.DNHeight - m.oneHourDNHeight) / oneHourMinutes
			m.follower.BVNRollingRate = float64(m.follower.BVNHeight - m.oneHourBVNHeight) / oneHourMinutes
		}
	} else {
		// After 2 hours: use 2-hour window
		twoHourElapsed := now.Sub(m.twoHourStartTime).Minutes()
		if twoHourElapsed >= 120 {
			// Reset 2-hour window
			m.twoHourStartTime = now
			m.twoHourDNHeight = m.follower.DNHeight
			m.twoHourBVNHeight = m.follower.BVNHeight
		}
		twoHourMinutes := now.Sub(m.twoHourStartTime).Minutes()
		if twoHourMinutes > 0 {
			m.follower.DNRollingRate = float64(m.follower.DNHeight - m.twoHourDNHeight) / twoHourMinutes
			m.follower.BVNRollingRate = float64(m.follower.BVNHeight - m.twoHourBVNHeight) / twoHourMinutes
		}
	}

	// Mainnet: Genesis rate and weekly rate (using block timestamps)
	m.calculateMainnetHistoricalRates()

	// Save current values for next calculation
	m.lastMainnetDN = m.mainnet.DNHeight
	m.lastMainnetBVN = m.mainnet.BVNHeight
	m.lastFollowerDN = m.follower.DNHeight
	m.lastFollowerBVN = m.follower.BVNHeight
	m.lastCheckTime = now
}

func (m *Monitor) calculateMainnetHistoricalRates() {
	// Genesis rate: From block 2 (first block with timestamp) to current
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

	// Weekly rate: From 7 days ago (14 major blocks) to current
	// Find block from ~7 days ago by estimating height (assuming ~60 blocks/min)
	estimatedBlocksInWeek := int64(60 * 60 * 24 * 7) // 60 blocks/min * 60 min * 24 hours * 7 days
	weekOldHeight := m.mainnet.DNHeight - estimatedBlocksInWeek
	if weekOldHeight < 2 {
		weekOldHeight = 2
	}

	if weekOldHeight > 0 {
		weekOldBlock, err := fetchBlock(mainnetDNRPC, weekOldHeight)
		if err == nil && weekOldBlock.Result.Block.Header.Time != "" {
			weekOldTime, err := parseTime(weekOldBlock.Result.Block.Header.Time)
			if err == nil {
				// Verify this block is actually close to 7 days old, adjust if needed
				actualDaysOld := time.Since(weekOldTime).Hours() / 24
				if actualDaysOld < 6.5 || actualDaysOld > 7.5 {
					// Block is not close enough to 7 days, search for better block
					// For now, use what we have
				}
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

	// Same for BVN
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

	// Check if follower is available
	if !m.follower.Available {
		alerts = append(alerts, Alert{
			Level:   "critical",
			Message: fmt.Sprintf("Follower is unavailable: %s", m.follower.Error),
			Time:    now,
		})
	}

	// Check if mainnet is available
	if !m.mainnet.Available {
		alerts = append(alerts, Alert{
			Level:   "critical",
			Message: "Mainnet is unavailable",
			Time:    now,
		})
	}

	if m.follower.Available && m.mainnet.Available {
		// Check DN lag
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

		// Check BVN lag
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

		// Check peer connections
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

		// Check if follower is syncing slower than expected
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

		// Success message when caught up
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

	// Fetch metrics from both nodes
	m.fetchNodeMetrics(m.mainnet, mainnetDNRPC, mainnetBVNRPC)
	m.fetchNodeMetrics(m.follower, followerDNRPC, followerBVNRPC)

	// Calculate rates
	m.calculateRates()

	// Calculate comparison metrics
	if m.mainnet.Available && m.follower.Available {
		m.comparison.DNLag = m.mainnet.DNHeight - m.follower.DNHeight
		m.comparison.BVNLag = m.mainnet.BVNHeight - m.follower.BVNHeight

		if m.mainnet.DNHeight > 0 {
			m.comparison.DNSyncPercent = (float64(m.follower.DNHeight) / float64(m.mainnet.DNHeight)) * 100
		}
		if m.mainnet.BVNHeight > 0 {
			m.comparison.BVNSyncPercent = (float64(m.follower.BVNHeight) / float64(m.mainnet.BVNHeight)) * 100
		}

		// Calculate ETA
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

	// Detect alerts
	m.detectAlerts()
}

func (m *Monitor) GetSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"mainnet":    m.mainnet,
		"follower":   m.follower,
		"comparison": m.comparison,
	}
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
	monitor := NewMonitor()

	// Initial update
	monitor.Update()

	// Start update loop
	go func() {
		ticker := time.NewTicker(10 * time.Second)
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

	log.Println("Follower Monitor starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
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
        .container { max-width: 1600px; margin: 0 auto; }
        header {
            background: #2a2a2a;
            padding: 8px 15px;
            border-radius: 4px;
            margin-bottom: 10px;
            border-left: 3px solid #4a9eff;
        }
        h1 { font-size: 18px; font-weight: 600; color: #fff; }
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
        .card {
            background: #2a2a2a;
            border-radius: 4px;
            padding: 10px;
            border-left: 3px solid #555;
        }
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
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Accumulate Follower Monitor</h1>
        </header>

        <div id="alerts" class="alerts"></div>

        <div class="grid">
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
                        <tr><td>Genesis</td><td id="mainnet-dn-genesis">-</td></tr>
                        <tr><td>Weekly</td><td id="mainnet-dn-weekly">-</td></tr>
                    </table>
                </div>
                <div class="section">
                    <div class="section-title">BVN Cyclops</div>
                    <table>
                        <tr><td>Height</td><td id="mainnet-bvn-height" class="value-large">-</td></tr>
                        <tr><td>Peers</td><td id="mainnet-bvn-peers">-</td></tr>
                        <tr><td>Current</td><td id="mainnet-bvn-rate">-</td></tr>
                        <tr><td>Genesis</td><td id="mainnet-bvn-genesis">-</td></tr>
                        <tr><td>Weekly</td><td id="mainnet-bvn-weekly">-</td></tr>
                    </table>
                </div>
            </div>

            <div class="card">
                <div class="card-title">
                    <span>Local Follower</span>
                    <span id="follower-status" class="status">...</span>
                </div>
                <div class="section">
                    <div class="section-title">Directory Network</div>
                    <table>
                        <tr><td>Height</td><td id="follower-dn-height" class="value-large">-</td></tr>
                        <tr><td>Peers</td><td id="follower-dn-peers">-</td></tr>
                        <tr><td>Current</td><td id="follower-dn-rate">-</td></tr>
                        <tr><td>Since Launch</td><td id="follower-dn-launch">-</td></tr>
                        <tr><td>Rolling</td><td id="follower-dn-rolling">-</td></tr>
                    </table>
                </div>
                <div class="section">
                    <div class="section-title">BVN Cyclops</div>
                    <table>
                        <tr><td>Height</td><td id="follower-bvn-height" class="value-large">-</td></tr>
                        <tr><td>Peers</td><td id="follower-bvn-peers">-</td></tr>
                        <tr><td>Current</td><td id="follower-bvn-rate">-</td></tr>
                        <tr><td>Since Launch</td><td id="follower-bvn-launch">-</td></tr>
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

        <div class="footer">
            Last updated: <span id="last-update">Never</span>
        </div>
    </div>

    <script>
        function fmt(num) {
            return new Intl.NumberFormat().format(num);
        }

        function fmtRate(rate) {
            if (rate === 0) return "0 bl/min";
            return Math.round(rate) + " bl/min";
        }

        function updateUI(data) {
            const { mainnet, follower, comparison } = data;

            if (mainnet.available) {
                document.getElementById('mainnet-status').textContent = 'Online';
                document.getElementById('mainnet-status').className = 'status online';
                document.getElementById('mainnet-dn-height').textContent = fmt(mainnet.dn_height);
                document.getElementById('mainnet-bvn-height').textContent = fmt(mainnet.bvn_height);
                document.getElementById('mainnet-dn-peers').textContent = mainnet.dn_peers;
                document.getElementById('mainnet-bvn-peers').textContent = mainnet.bvn_peers;
                document.getElementById('mainnet-dn-rate').textContent = fmtRate(mainnet.dn_block_rate);
                document.getElementById('mainnet-bvn-rate').textContent = fmtRate(mainnet.bvn_block_rate);
                document.getElementById('mainnet-dn-genesis').textContent = fmtRate(mainnet.dn_genesis_rate);
                document.getElementById('mainnet-bvn-genesis').textContent = fmtRate(mainnet.bvn_genesis_rate);
                document.getElementById('mainnet-dn-weekly').textContent = fmtRate(mainnet.dn_weekly_rate);
                document.getElementById('mainnet-bvn-weekly').textContent = fmtRate(mainnet.bvn_weekly_rate);
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
                document.getElementById('follower-dn-launch').textContent = fmtRate(follower.dn_since_launch);
                document.getElementById('follower-bvn-launch').textContent = fmtRate(follower.bvn_since_launch);
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

        async function fetchMetrics() {
            try {
                const response = await fetch('/api/metrics');
                const data = await response.json();
                updateUI(data);
            } catch (error) {
                console.error('Failed to fetch metrics:', error);
            }
        }

        fetchMetrics();
        setInterval(fetchMetrics, 10000);
    </script>
</body>
</html>`

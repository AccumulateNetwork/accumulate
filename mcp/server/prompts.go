package server

import (
	"fmt"
	"strings"
)

// PromptArgument defines a prompt argument
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// PromptDefinition defines an MCP prompt template
type PromptDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// GetAllPrompts returns all available prompt templates
func GetAllPrompts() []PromptDefinition {
	return []PromptDefinition{
		{
			Name:        "deploy-follower-node",
			Description: "Complete workflow for deploying an Accumulate follower node from database snapshots",
			Arguments: []PromptArgument{
				{
					Name:        "dn_database",
					Description: "Path to Directory Network database snapshot",
					Required:    true,
				},
				{
					Name:        "bvn_database",
					Description: "Path to Block Validation Network database snapshot",
					Required:    true,
				},
				{
					Name:        "work_dir",
					Description: "Directory for follower configuration and data",
					Required:    true,
				},
				{
					Name:        "peer_url",
					Description: "Peer BVN URL to connect to (e.g., tcp://peer.example.com:16691)",
					Required:    false,
				},
				{
					Name:        "seed_proxy",
					Description: "Seed proxy URL for network configuration",
					Required:    false,
				},
				{
					Name:        "public_ip",
					Description: "Follower's public IP address",
					Required:    false,
				},
			},
		},
		{
			Name:        "monitor-follower-health",
			Description: "Monitor health and sync status of Accumulate follower node",
			Arguments: []PromptArgument{
				{
					Name:        "work_dir",
					Description: "Follower working directory",
					Required:    false,
				},
				{
					Name:        "endpoint",
					Description: "Follower API endpoint (if different from default)",
					Required:    false,
				},
			},
		},
		{
			Name:        "troubleshoot-follower-sync",
			Description: "Diagnose and resolve follower node synchronization issues",
			Arguments: []PromptArgument{
				{
					Name:        "work_dir",
					Description: "Follower working directory",
					Required:    false,
				},
				{
					Name:        "symptom",
					Description: "Observed issue: no_peers, not_syncing, slow_sync, or crashed",
					Required:    false,
				},
			},
		},
		{
			Name:        "setup-dev-wallet",
			Description: "Set up Accumulate wallet for development with testnet tokens",
			Arguments: []PromptArgument{
				{
					Name:        "network",
					Description: "Network to use: testnet or devnet (default: testnet)",
					Required:    false,
				},
				{
					Name:        "wallet_dir",
					Description: "Wallet directory path",
					Required:    false,
				},
				{
					Name:        "no_password",
					Description: "Initialize without password for development",
					Required:    false,
				},
			},
		},
		{
			Name:        "quick-node-status",
			Description: "Quick status check for follower node (concise output)",
			Arguments: []PromptArgument{
				{
					Name:        "work_dir",
					Description: "Follower working directory",
					Required:    false,
				},
			},
		},
		{
			Name:        "organize-documentation",
			Description: "Organize and manage project documentation following Accumulate standards",
			Arguments: []PromptArgument{
				{
					Name:        "action",
					Description: "Action to perform: review, organize, archive, or cleanup",
					Required:    true,
				},
				{
					Name:        "scope",
					Description: "Scope of operation: all, new-files, or specific directory path",
					Required:    true,
				},
				{
					Name:        "create_index",
					Description: "Create or update documentation index (default: true)",
					Required:    false,
				},
				{
					Name:        "dry_run",
					Description: "Show what would be done without making changes (default: false)",
					Required:    false,
				},
			},
		},
		{
			Name:        "prepare-mainnet-follower",
			Description: "Complete pre-deployment preparation for mainnet follower including prerequisites validation and snapshot verification",
			Arguments: []PromptArgument{
				{
					Name:        "work_dir",
					Description: "Target working directory for follower deployment",
					Required:    true,
				},
				{
					Name:        "bvn",
					Description: "BVN to follow: Cyclops, Apollo, Yutu, or Chandrayaan (default: Cyclops)",
					Required:    false,
				},
				{
					Name:        "dn_database",
					Description: "Path to DN database snapshot (if already available)",
					Required:    false,
				},
				{
					Name:        "bvn_database",
					Description: "Path to BVN database snapshot (if already available)",
					Required:    false,
				},
			},
		},
		{
			Name:        "recovery-from-failure",
			Description: "Diagnose and recover from follower node failure with guided steps",
			Arguments: []PromptArgument{
				{
					Name:        "container_name",
					Description: "Docker container name (default: accumulate-follower)",
					Required:    false,
				},
				{
					Name:        "failure_type",
					Description: "Type of failure: crashed, sync_stalled, no_peers, db_corruption, unknown",
					Required:    false,
				},
				{
					Name:        "work_dir",
					Description: "Follower working directory",
					Required:    false,
				},
			},
		},
		{
			Name:        "mainnet-sync-status",
			Description: "Quick comparison of local follower sync status against mainnet",
			Arguments: []PromptArgument{
				{
					Name:        "container_name",
					Description: "Docker container name (default: accumulate-follower)",
					Required:    false,
				},
			},
		},
	}
}

// GetPromptTemplate generates the template content for a specific prompt with arguments
func GetPromptTemplate(name string, args map[string]string) (string, error) {
	// Helper function to get arg with default
	getArg := func(key, defaultValue string) string {
		if val, ok := args[key]; ok && val != "" {
			return val
		}
		return defaultValue
	}

	switch name {
	case "deploy-follower-node":
		return generateDeployFollowerNodeTemplate(args, getArg), nil
	case "monitor-follower-health":
		return generateMonitorFollowerHealthTemplate(args, getArg), nil
	case "troubleshoot-follower-sync":
		return generateTroubleshootFollowerSyncTemplate(args, getArg), nil
	case "setup-dev-wallet":
		return generateSetupDevWalletTemplate(args, getArg), nil
	case "quick-node-status":
		return generateQuickNodeStatusTemplate(args, getArg), nil
	case "organize-documentation":
		return generateOrganizeDocumentationTemplate(args, getArg), nil
	case "prepare-mainnet-follower":
		return generatePrepareMainnetFollowerTemplate(args, getArg), nil
	case "recovery-from-failure":
		return generateRecoveryFromFailureTemplate(args, getArg), nil
	case "mainnet-sync-status":
		return generateMainnetSyncStatusTemplate(args, getArg), nil
	default:
		return "", fmt.Errorf("unknown prompt: %s", name)
	}
}

// ValidatePromptArguments validates required arguments for a prompt
func ValidatePromptArguments(name string, args map[string]string) error {
	prompts := GetAllPrompts()
	for _, prompt := range prompts {
		if prompt.Name == name {
			for _, arg := range prompt.Arguments {
				if arg.Required {
					if _, ok := args[arg.Name]; !ok {
						return fmt.Errorf("missing required argument: %s", arg.Name)
					}
				}
			}
			return nil
		}
	}
	return fmt.Errorf("prompt not found: %s", name)
}

func generateDeployFollowerNodeTemplate(args map[string]string, getArg func(string, string) string) string {
	dnDatabase := args["dn_database"]
	bvnDatabase := args["bvn_database"]
	workDir := args["work_dir"]
	peerURL := getArg("peer_url", "")
	seedProxy := getArg("seed_proxy", "")
	publicIP := getArg("public_ip", "")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Deploy Accumulate Follower Node\n\n"))
	b.WriteString(fmt.Sprintf("**Target Directory:** %s\n\n", workDir))

	b.WriteString("## Step 1: Validate Prerequisites\n\n")
	b.WriteString("First, run `accumulate_validate_prerequisites` to check system requirements:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s",`+"\n", workDir))
	b.WriteString(`  "network": "mainnet"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Required Checks:**\n")
	b.WriteString("- [ ] Disk space: 100+ GB free\n")
	b.WriteString("- [ ] Memory: 8+ GB RAM\n")
	b.WriteString("- [ ] Docker: Installed and running\n")
	b.WriteString("- [ ] Ports 16591-16593, 16691-16693: Available\n")
	b.WriteString("- [ ] Network: Bootstrap server reachable\n\n")

	b.WriteString("**If prerequisites fail**, address the issues before proceeding.\n\n")

	b.WriteString("## Step 2: Verify Database Snapshots\n\n")
	b.WriteString("**Database Snapshots:**\n")
	b.WriteString(fmt.Sprintf("- DN: `%s`\n", dnDatabase))
	b.WriteString(fmt.Sprintf("- BVN: `%s`\n\n", bvnDatabase))

	b.WriteString("Verify snapshots exist and are valid:\n")
	b.WriteString(fmt.Sprintf("- Check `%s/data/accumulate.db` exists\n", dnDatabase))
	b.WriteString(fmt.Sprintf("- Check `%s/data/accumulate.db` exists\n\n", bvnDatabase))

	b.WriteString("## Step 3: Initialize Follower\n\n")
	b.WriteString("Use `accumulate_init_follower` with automatic peer discovery:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "dn_database": "%s",`+"\n", dnDatabase))
	b.WriteString(fmt.Sprintf(`  "bvn_database": "%s",`+"\n", bvnDatabase))
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s",`+"\n", workDir))
	b.WriteString(`  "auto_discover_peers": true`)
	if peerURL != "" {
		b.WriteString(fmt.Sprintf(`,`+"\n"+`  "peer_url": "%s"`, peerURL))
	}
	if seedProxy != "" {
		b.WriteString(fmt.Sprintf(`,`+"\n"+`  "seed_proxy": "%s"`, seedProxy))
	}
	if publicIP != "" {
		b.WriteString(fmt.Sprintf(`,`+"\n"+`  "public_ip": "%s"`, publicIP))
	}
	b.WriteString("\n}\n```\n\n")

	b.WriteString("**Expected Output:**\n")
	b.WriteString("- Status: `initialized`\n")
	b.WriteString("- peer_source: `queried from bootstrap server` or `hardcoded defaults`\n")
	b.WriteString(fmt.Sprintf("- DN copied to: `%s/dnn`\n", workDir))
	b.WriteString(fmt.Sprintf("- BVN copied to: `%s/bvnn`\n\n", workDir))

	b.WriteString("## Step 4: Start Follower\n\n")
	b.WriteString("Use `accumulate_run_follower`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s"`+"\n", workDir))
	b.WriteString("}\n```\n\n")

	b.WriteString("**Expected Output:**\n")
	b.WriteString("- Status: `started`\n")
	b.WriteString("- container_id: [Docker container ID]\n")
	b.WriteString("- ports: DN 16591-16593, BVN 16691-16693\n\n")

	b.WriteString("## Step 5: Verify Startup (Wait 30-60 seconds)\n\n")
	b.WriteString("Use `accumulate_follower_status`:\n")
	b.WriteString("```json\n")
	b.WriteString("{}\n```\n\n")

	b.WriteString("**Check for:**\n")
	b.WriteString("- [ ] Container running: YES\n")
	b.WriteString("- [ ] No critical errors in stats\n\n")

	b.WriteString("## Step 6: Monitor Sync Progress\n\n")
	b.WriteString("Use `accumulate_get_sync_progress` for detailed sync status:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "include_rate": true`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Monitor:**\n")
	b.WriteString("- sync_percentage: Should increase over time\n")
	b.WriteString("- blocks_behind: Should decrease\n")
	b.WriteString("- peer_count: Should be 3+ within 5 minutes\n")
	b.WriteString("- estimated_eta: Time to full sync\n\n")

	b.WriteString("## Step 7: Analyze Logs (If Issues)\n\n")
	b.WriteString("Use `accumulate_analyze_logs` to diagnose problems:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "lines": 500,`+"\n")
	b.WriteString(`  "filter": "all"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Check:**\n")
	b.WriteString("- status: Should be `healthy`\n")
	b.WriteString("- error_count: Should be 0 or low\n")
	b.WriteString("- recommendations: Follow any suggestions\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Expected Timeline\n\n")
	b.WriteString("| Phase | Time |\n")
	b.WriteString("|-------|------|\n")
	b.WriteString("| Startup | 1-2 minutes |\n")
	b.WriteString("| First peers | 2-5 minutes |\n")
	b.WriteString("| Sync begins | 5-10 minutes |\n")
	b.WriteString("| Full sync | 2-24 hours (depends on snapshot age) |\n\n")

	b.WriteString("## Troubleshooting Quick Reference\n\n")
	b.WriteString("| Issue | Tool | Action |\n")
	b.WriteString("|-------|------|--------|\n")
	b.WriteString("| Prerequisites failed | `accumulate_validate_prerequisites` | Address each failed check |\n")
	b.WriteString("| No peers | `accumulate_analyze_logs` | Check firewall, verify bootstrap server |\n")
	b.WriteString("| Sync stalled | `accumulate_get_sync_progress` | Check peer count, restart if needed |\n")
	b.WriteString("| Container crashed | `accumulate_analyze_logs` | Review critical errors, re-deploy |\n\n")

	b.WriteString("## Monitoring Commands\n\n")
	b.WriteString("**Quick status check:**\n")
	b.WriteString("1. `accumulate_follower_status` - Is it running?\n")
	b.WriteString("2. `accumulate_get_sync_progress` - How far behind?\n")
	b.WriteString("3. `accumulate_analyze_logs` - Any errors?\n\n")

	b.WriteString("## Related Prompts\n\n")
	b.WriteString("- `monitor-follower-health` - Ongoing health monitoring\n")
	b.WriteString("- `troubleshoot-follower-sync` - Detailed sync troubleshooting\n")
	b.WriteString("- `quick-node-status` - Fast status check\n")

	return b.String()
}

func generateMonitorFollowerHealthTemplate(args map[string]string, getArg func(string, string) string) string {
	workDir := getArg("work_dir", "~/.accumulate/follower")
	endpoint := getArg("endpoint", "mainnet")

	var b strings.Builder
	b.WriteString("# Monitor Accumulate Follower Health\n\n")

	b.WriteString("## Step 1: Check Container Status\n\n")
	b.WriteString("Use `accumulate_follower_status`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s"`+"\n", workDir))
	b.WriteString("}\n```\n\n")

	b.WriteString("**Expected Output:**\n")
	b.WriteString("- `running`: true/false\n")
	b.WriteString("- `container_id`: [ID if running]\n")
	b.WriteString("- `uptime`: [duration]\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Step 2: Get Sync Progress (NEW)\n\n")
	b.WriteString("Use `accumulate_get_sync_progress` for comprehensive sync status:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "include_rate": true`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Key Metrics:**\n")
	b.WriteString("| Metric | Healthy | Warning | Critical |\n")
	b.WriteString("|--------|---------|---------|----------|\n")
	b.WriteString("| `sync_percentage` | > 99% | 90-99% | < 90% |\n")
	b.WriteString("| `blocks_behind` | < 100 | 100-1000 | > 1000 |\n")
	b.WriteString("| `peer_count` | 3+ | 1-2 | 0 |\n")
	b.WriteString("| `sync_rate_blocks` | > 50/min | 10-50/min | < 10/min |\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Step 3: Analyze Logs (NEW)\n\n")
	b.WriteString("Use `accumulate_analyze_logs` for error detection:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "lines": 500,`+"\n")
	b.WriteString(`  "filter": "all"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Health Assessment:**\n")
	b.WriteString("| Log Status | Meaning |\n")
	b.WriteString("|------------|--------|\n")
	b.WriteString("| `healthy` | No significant issues |\n")
	b.WriteString("| `degraded` | Errors present, may need attention |\n")
	b.WriteString("| `critical` | Immediate action required |\n\n")

	b.WriteString("**Key Patterns to Watch:**\n")
	b.WriteString("- `panic`: Application crash\n")
	b.WriteString("- `database`: Storage issues\n")
	b.WriteString("- `connection`: Network problems\n")
	b.WriteString("- `peers`: Peer discovery failures\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Step 4: Compare with Network\n\n")
	b.WriteString("Use `accumulate_network_status` for reference:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "network": "%s"`+"\n", endpoint))
	b.WriteString("}\n```\n\n")

	b.WriteString("Compare your follower's height with the network height.\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Health Status Summary\n\n")

	b.WriteString("### HEALTHY\n")
	b.WriteString("- Container running\n")
	b.WriteString("- Sync percentage > 99%\n")
	b.WriteString("- Peers >= 3\n")
	b.WriteString("- Log status: `healthy`\n")
	b.WriteString("- **Action:** Continue monitoring\n\n")

	b.WriteString("### WARNING\n")
	b.WriteString("- Sync percentage 90-99%\n")
	b.WriteString("- Peers 1-2\n")
	b.WriteString("- Log status: `degraded`\n")
	b.WriteString("- **Action:** Monitor closely, check again in 15 min\n\n")

	b.WriteString("### CRITICAL\n")
	b.WriteString("- Container stopped\n")
	b.WriteString("- Peers = 0\n")
	b.WriteString("- Sync stalled\n")
	b.WriteString("- Log status: `critical`\n")
	b.WriteString("- **Action:** Use `recovery-from-failure` prompt\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Quick Actions\n\n")
	b.WriteString("| Status | Action |\n")
	b.WriteString("|--------|--------|\n")
	b.WriteString("| HEALTHY | Check again in 30 minutes |\n")
	b.WriteString("| WARNING | Use `troubleshoot-follower-sync` |\n")
	b.WriteString("| CRITICAL | Use `recovery-from-failure` |\n\n")

	b.WriteString("## Related Prompts\n\n")
	b.WriteString("- `quick-node-status` - Fast status check\n")
	b.WriteString("- `mainnet-sync-status` - Sync comparison\n")
	b.WriteString("- `troubleshoot-follower-sync` - Diagnose issues\n")
	b.WriteString("- `recovery-from-failure` - Recovery procedures\n")

	return b.String()
}

func generateTroubleshootFollowerSyncTemplate(args map[string]string, getArg func(string, string) string) string {
	workDir := getArg("work_dir", "~/.accumulate/follower")
	symptom := getArg("symptom", "general")

	var b strings.Builder
	b.WriteString("# Troubleshoot Accumulate Follower Sync Issues\n\n")
	if symptom != "general" {
		b.WriteString(fmt.Sprintf("**Reported Symptom:** %s\n\n", symptom))
	}

	b.WriteString("---\n\n")

	b.WriteString("## Step 1: Automated Diagnostics\n\n")
	b.WriteString("Run these tools to gather diagnostic information:\n\n")

	b.WriteString("### 1.1 Container Status\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_follower_status {\n")
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s"`+"\n", workDir))
	b.WriteString("}\n```\n\n")

	b.WriteString("### 1.2 Sync Progress (NEW)\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_get_sync_progress {\n")
	b.WriteString(`  "include_rate": true`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Interpretation:**\n")
	b.WriteString("| Field | Healthy | Problem |\n")
	b.WriteString("|-------|---------|--------|\n")
	b.WriteString("| `status` | syncing/synced | stalled/stopped |\n")
	b.WriteString("| `peer_count` | 3+ | 0-2 |\n")
	b.WriteString("| `blocks_behind` | Decreasing | Static/increasing |\n")
	b.WriteString("| `sync_rate_blocks` | > 10/min | < 10/min |\n\n")

	b.WriteString("### 1.3 Log Analysis (NEW)\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_analyze_logs {\n")
	b.WriteString(`  "lines": 1000,`+"\n")
	b.WriteString(`  "filter": "all"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Key Patterns:**\n")
	b.WriteString("| Pattern | Meaning | Severity |\n")
	b.WriteString("|---------|---------|----------|\n")
	b.WriteString("| `panic` | Application crash | Critical |\n")
	b.WriteString("| `database` | Storage error | Error |\n")
	b.WriteString("| `connection` | Network issue | Error |\n")
	b.WriteString("| `peers` | Peer discovery | Warning |\n")
	b.WriteString("| `timeout` | Operation timeout | Error |\n\n")

	b.WriteString("### 1.4 Prerequisites Check (if needed)\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_validate_prerequisites {\n")
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s",`+"\n", workDir))
	b.WriteString(`  "network": "mainnet"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Step 2: Identify Issue Category\n\n")
	b.WriteString("Based on diagnostics, identify the primary issue:\n\n")

	b.WriteString("| Diagnostic Result | Issue Category | Go to Section |\n")
	b.WriteString("|------------------|----------------|---------------|\n")
	b.WriteString("| Container stopped | **Crashed** | Section A |\n")
	b.WriteString("| peer_count = 0 | **No Peers** | Section B |\n")
	b.WriteString("| status = stalled | **Sync Stalled** | Section C |\n")
	b.WriteString("| sync_rate < 10/min | **Slow Sync** | Section D |\n")
	b.WriteString("| database errors | **DB Issues** | Section E |\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Section A: Container Crashed\n\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- `accumulate_follower_status`: container not running\n")
	b.WriteString("- `accumulate_analyze_logs`: shows `panic` or `fatal`\n\n")

	b.WriteString("**Common Causes:**\n")
	b.WriteString("1. Out of memory (OOM)\n")
	b.WriteString("2. Disk full\n")
	b.WriteString("3. Database corruption\n")
	b.WriteString("4. Software bug\n\n")

	b.WriteString("**Resolution:**\n")
	b.WriteString("1. Check log analysis `recommendations` field\n")
	b.WriteString("2. Check system resources:\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   free -h  # Memory\n")
	b.WriteString(fmt.Sprintf("   df -h %s  # Disk\n", workDir))
	b.WriteString("   ```\n")
	b.WriteString("3. If OOM: increase memory or add swap\n")
	b.WriteString("4. If disk full: free space\n")
	b.WriteString("5. Restart:\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_run_follower {\n")
	b.WriteString(fmt.Sprintf(`     "work_dir": "%s"`+"\n", workDir))
	b.WriteString("   }\n")
	b.WriteString("   ```\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Section B: No Peers\n\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- `accumulate_get_sync_progress`: peer_count = 0\n")
	b.WriteString("- `accumulate_analyze_logs`: connection errors\n\n")

	b.WriteString("**Common Causes:**\n")
	b.WriteString("1. Firewall blocking ports\n")
	b.WriteString("2. Network connectivity\n")
	b.WriteString("3. Bootstrap server unreachable\n")
	b.WriteString("4. Invalid peer configuration\n\n")

	b.WriteString("**Resolution:**\n")
	b.WriteString("1. Check bootstrap server:\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_query_bootstrap_server {}\n")
	b.WriteString("   ```\n")
	b.WriteString("2. Verify firewall allows ports 16591-16593, 16691-16693:\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   sudo ufw status\n")
	b.WriteString("   # Or\n")
	b.WriteString("   sudo ufw allow 16591:16693/tcp\n")
	b.WriteString("   ```\n")
	b.WriteString("3. Re-initialize with auto peer discovery:\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_init_follower {\n")
	b.WriteString(fmt.Sprintf(`     "work_dir": "%s",`+"\n", workDir))
	b.WriteString(`     "auto_discover_peers": true`+"\n")
	b.WriteString("   }\n")
	b.WriteString("   ```\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Section C: Sync Stalled\n\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- `accumulate_get_sync_progress`: status = stalled\n")
	b.WriteString("- blocks_behind not decreasing over time\n")
	b.WriteString("- peer_count may be > 0\n\n")

	b.WriteString("**Common Causes:**\n")
	b.WriteString("1. Corrupted block data\n")
	b.WriteString("2. Incompatible snapshot\n")
	b.WriteString("3. Network partition\n\n")

	b.WriteString("**Resolution:**\n")
	b.WriteString("1. Restart the follower:\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_stop_follower {}\n")
	b.WriteString("   // Wait 10 seconds\n")
	b.WriteString("   accumulate_run_follower {\n")
	b.WriteString(fmt.Sprintf(`     "work_dir": "%s"`+"\n", workDir))
	b.WriteString("   }\n")
	b.WriteString("   ```\n")
	b.WriteString("2. Monitor for 10 minutes\n")
	b.WriteString("3. If still stalled, consider re-deploy with fresh snapshots\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Section D: Slow Sync\n\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- `accumulate_get_sync_progress`: sync_rate < 10 blocks/min\n")
	b.WriteString("- Syncing but ETA is very long\n\n")

	b.WriteString("**Common Causes:**\n")
	b.WriteString("1. Limited peer connections\n")
	b.WriteString("2. Slow storage (HDD vs SSD)\n")
	b.WriteString("3. Resource constraints\n")
	b.WriteString("4. Network bandwidth\n\n")

	b.WriteString("**Resolution:**\n")
	b.WriteString("1. Check prerequisites:\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_validate_prerequisites {\n")
	b.WriteString(fmt.Sprintf(`     "work_dir": "%s"`+"\n", workDir))
	b.WriteString("   }\n")
	b.WriteString("   ```\n")
	b.WriteString("2. If CPU/memory warnings, upgrade resources\n")
	b.WriteString("3. If disk I/O slow, use SSD\n")
	b.WriteString("4. If few peers, check network/firewall\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Section E: Database Issues\n\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- `accumulate_analyze_logs`: database errors in patterns\n")
	b.WriteString("- Repeated crashes with DB-related messages\n\n")

	b.WriteString("**Resolution:**\n")
	b.WriteString("1. This usually requires a fresh deployment\n")
	b.WriteString("2. Stop and remove follower:\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_remove_follower {}\n")
	b.WriteString("   ```\n")
	b.WriteString("3. Clear work directory\n")
	b.WriteString("4. Re-deploy with fresh snapshots using `deploy-follower-node`\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Post-Resolution Verification\n\n")
	b.WriteString("After applying fixes, verify recovery:\n\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_get_sync_progress { \"include_rate\": true }\n")
	b.WriteString("```\n\n")

	b.WriteString("**Success Criteria:**\n")
	b.WriteString("- status: `syncing` or `synced`\n")
	b.WriteString("- peer_count: 3+\n")
	b.WriteString("- sync_rate: > 10 blocks/min (if syncing)\n")
	b.WriteString("- blocks_behind: Decreasing\n\n")

	b.WriteString("## Related Prompts\n\n")
	b.WriteString("- `recovery-from-failure` - Guided recovery\n")
	b.WriteString("- `monitor-follower-health` - Ongoing monitoring\n")
	b.WriteString("- `deploy-follower-node` - Fresh deployment\n")
	b.WriteString("- `mainnet-sync-status` - Quick sync check\n")

	return b.String()
}

func generateSetupDevWalletTemplate(args map[string]string, getArg func(string, string) string) string {
	network := getArg("network", "testnet")
	walletDir := getArg("wallet_dir", "~/.accumulate/wallet")
	noPassword := getArg("no_password", "false")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Setup development wallet for network: %s\n\n", network))

	b.WriteString("**Steps:**\n\n")

	b.WriteString("**1. Initialize Wallet**\n")
	b.WriteString("Use `wallet_init`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "wallet_dir": "%s"`, walletDir))
	if noPassword == "true" {
		b.WriteString(`,`+"\n"+`  "no_password": true`)
	}
	b.WriteString("\n}\n```\n\n")

	b.WriteString("**Expected Output:**\n")
	b.WriteString("- Status: \"initialized\"\n")
	b.WriteString(fmt.Sprintf("- Wallet directory: %s\n", walletDir))
	b.WriteString("- Vault created\n\n")

	b.WriteString("**2. Open Vault**\n")
	b.WriteString("Use `wallet_vault_open`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	if noPassword != "true" {
		b.WriteString(`  "password": "your-password"`+"\n")
	}
	b.WriteString("}\n```\n\n")

	b.WriteString("**3. Generate Keys**\n")
	b.WriteString("Generate 2-3 keys for development:\n")
	b.WriteString("Use `wallet_generate_key` (repeat 2-3 times):\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "label": "dev-key-1"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Expected Output:**\n")
	b.WriteString("- Public key: [hex string]\n")
	b.WriteString("- Label: dev-key-1\n")
	b.WriteString("- Key stored in vault\n\n")

	b.WriteString("**4. Set Network**\n")
	b.WriteString("Configure wallet for target network:\n")
	b.WriteString("Use `wallet_set_network`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "network": "%s"`+"\n", network))
	b.WriteString("}\n```\n\n")

	b.WriteString("**5. Create Lite Account**\n")
	b.WriteString("Create a lite account for testing:\n")
	b.WriteString("Use `accumulate_create_lite_account`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "public_key": "[key from step 3]",`+"\n")
	b.WriteString(fmt.Sprintf(`  "network": "%s"`+"\n", network))
	b.WriteString("}\n```\n\n")

	b.WriteString("**Expected Output:**\n")
	b.WriteString("- Lite account URL: acc://[hash]/ACME\n")
	b.WriteString("- Initial balance: 0 ACME\n\n")

	if network == "testnet" || network == "devnet" {
		b.WriteString("**6. Get Testnet Tokens**\n")
		b.WriteString("Use faucet to get test ACME tokens:\n")
		b.WriteString("Use `accumulate_faucet`:\n")
		b.WriteString("```json\n")
		b.WriteString("{\n")
		b.WriteString(`  "account": "[lite account from step 5]"`+"\n")
		b.WriteString("}\n```\n\n")

		b.WriteString("**Expected Output:**\n")
		b.WriteString("- Faucet transaction sent\n")
		b.WriteString("- Wait 5-10 seconds for confirmation\n")
		b.WriteString("- Balance should increase to ~10 ACME\n\n")
	}

	b.WriteString("**7. Verify Wallet Status**\n")
	b.WriteString("Use `wallet_get_status` to verify setup:\n")
	b.WriteString("```json\n")
	b.WriteString("{}\n```\n\n")

	b.WriteString("**Expected Output:**\n")
	b.WriteString("- Vault: open\n")
	b.WriteString(fmt.Sprintf("- Network: %s\n", network))
	b.WriteString("- Keys: 2-3 keys listed\n")
	b.WriteString("- Ready for development\n\n")

	b.WriteString("**Validation Checklist:**\n")
	b.WriteString("- [ ] Wallet initialized\n")
	b.WriteString("- [ ] Vault opened\n")
	b.WriteString("- [ ] Keys generated (2-3)\n")
	b.WriteString(fmt.Sprintf("- [ ] Network set to %s\n", network))
	b.WriteString("- [ ] Lite account created\n")
	if network == "testnet" || network == "devnet" {
		b.WriteString("- [ ] Faucet tokens received\n")
	}
	b.WriteString("- [ ] Wallet status verified\n\n")

	b.WriteString("**Next Steps:**\n\n")
	b.WriteString("Your wallet is ready for development!\n\n")
	b.WriteString("You can now:\n")
	b.WriteString("- Create ADIs with `accumulate_create_adi`\n")
	b.WriteString("- Send tokens with `accumulate_send_tokens`\n")
	b.WriteString("- Create token accounts with `accumulate_create_token_account`\n")
	b.WriteString("- Write data with `accumulate_write_data`\n\n")

	b.WriteString("**Common Commands:**\n")
	b.WriteString("- List keys: `wallet_list_keys`\n")
	b.WriteString("- Query account: `accumulate_query_account`\n")
	b.WriteString("- Check balance: Query your lite account\n\n")

	b.WriteString("**Security Note:**\n")
	if noPassword == "true" {
		b.WriteString("⚠️ No password protection - FOR DEVELOPMENT ONLY!\n")
		b.WriteString("Never use no_password for production wallets.\n\n")
	} else {
		b.WriteString("Remember your wallet password - it cannot be recovered!\n\n")
	}

	b.WriteString("**Related Prompts:**\n")
	b.WriteString("- `create-adi-with-accounts` - Create ADI structure\n")
	b.WriteString("- `deploy-follower-node` - Run local node\n")

	return b.String()
}

func generateQuickNodeStatusTemplate(args map[string]string, getArg func(string, string) string) string {
	workDir := getArg("work_dir", "~/.accumulate/follower")

	var b strings.Builder
	b.WriteString("Quick Node Status Check\n\n")

	b.WriteString("**Step 1: Process Status**\n")
	b.WriteString("Use `accumulate_follower_status`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s"`+"\n", workDir))
	b.WriteString("}\n```\n\n")

	b.WriteString("**Step 2: Node Info**\n")
	b.WriteString("Use `accumulate_node_info`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "network": "mainnet"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Concise Output:**\n\n")
	b.WriteString("```\n")
	b.WriteString("📊 Process: [RUNNING/STOPPED]\n")
	b.WriteString("🔗 Peers: [count]\n")
	b.WriteString("📦 Block: [height] (behind: [diff])\n")
	b.WriteString("⏱️ Uptime: [duration]\n")
	b.WriteString("✅ Status: [HEALTHY/WARNING/ERROR]\n")
	b.WriteString("```\n\n")

	b.WriteString("**Quick Health Assessment:**\n\n")
	b.WriteString("✅ **HEALTHY**: Process running, 3+ peers, <100 blocks behind\n")
	b.WriteString("⚠️ **WARNING**: 1-2 peers or 100-1000 blocks behind\n")
	b.WriteString("❌ **ERROR**: Not running or 0 peers or >1000 blocks behind\n\n")

	b.WriteString("**Actions:**\n\n")
	b.WriteString("If not HEALTHY:\n")
	b.WriteString("- Use `monitor-follower-health` for detailed check\n")
	b.WriteString("- Use `troubleshoot-follower-sync` if issues found\n\n")

	b.WriteString("**Related Prompts:**\n")
	b.WriteString("- `monitor-follower-health` - Detailed health check\n")
	b.WriteString("- `troubleshoot-follower-sync` - Diagnose issues\n")

	return b.String()
}

func generateOrganizeDocumentationTemplate(args map[string]string, getArg func(string, string) string) string {
	action := getArg("action", "review")
	scope := getArg("scope", "all")
	createIndex := getArg("create_index", "true")
	dryRun := getArg("dry_run", "false")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Documentation Management: %s (%s)\n\n", action, scope))

	if dryRun == "true" {
		b.WriteString("🔍 **DRY RUN MODE** - No changes will be made\n\n")
	}

	b.WriteString("## Step 1: Review Current State\n\n")
	b.WriteString("List all markdown files in the repository:\n")
	b.WriteString("```bash\n")
	b.WriteString("find . -name '*.md' -type f | grep -v node_modules | sort\n")
	b.WriteString("```\n\n")

	b.WriteString("Identify documentation that needs organization:\n")
	b.WriteString("- [ ] Files in root directory (except README.md, CHANGELOG.md, CONTRIBUTING.md)\n")
	b.WriteString("- [ ] Files in inappropriate locations (cmd/, mcp/, tools/ with all-caps names)\n")
	b.WriteString("- [ ] Development session notes\n")
	b.WriteString("- [ ] Test results\n\n")

	b.WriteString("## Step 2: Categorize Documentation\n\n")
	b.WriteString("**Active Documentation:**\n")
	b.WriteString("- Guides → `docs/guides/`\n")
	b.WriteString("- Architecture → `docs/architecture/`\n")
	b.WriteString("- Design → `docs/design/`\n")
	b.WriteString("- Network/Deployment → `docs/network/` or `docs/deployment/`\n")
	b.WriteString("- API → `docs/api/`\n")
	b.WriteString("- Tutorials → `docs/tutorials/`\n\n")

	b.WriteString("**Archive Documentation:**\n")
	b.WriteString("- Development sessions → `docs/archive/development/YYYY-MM-DD-description.md`\n")
	b.WriteString("- Test results → `docs/archive/testing/YYYY-MM-DD-description.md`\n")
	b.WriteString("- Meeting notes → `docs/archive/meetings/YYYY-MM-DD-description.md`\n")
	b.WriteString("- Investigation reports → `docs/archive/investigations/YYYY-MM-DD-description.md`\n\n")

	b.WriteString("**Delete:**\n")
	b.WriteString("- Content fully integrated into other docs\n")
	b.WriteString("- Completely obsolete information\n")
	b.WriteString("- Duplicate files\n\n")

	b.WriteString("## Step 3: Apply Filename Standards\n\n")
	b.WriteString("**Rules:**\n")
	b.WriteString("1. Use lowercase with hyphens: `my-document.md`\n")
	b.WriteString("2. NO all-caps names (except README.md, CHANGELOG.md, CONTRIBUTING.md)\n")
	b.WriteString("3. Archive files include dates: `YYYY-MM-DD-description.md`\n")
	b.WriteString("4. Descriptive names that indicate content\n")
	b.WriteString("5. Avoid abbreviations unless widely known\n\n")

	b.WriteString("**Examples:**\n")
	b.WriteString("- ❌ `BADGER_VALIDATION_INVESTIGATION.md`\n")
	b.WriteString("- ✅ `2025-10-27-badger-validation-investigation.md`\n\n")
	b.WriteString("- ❌ `CONFIG_VALIDATION.md`\n")
	b.WriteString("- ✅ `configuration-validation.md`\n\n")

	b.WriteString("## Step 4: Create Directory Structure\n\n")
	b.WriteString("```bash\n")
	b.WriteString("mkdir -p docs/{guides,architecture,design,network,deployment,api,tutorials}\n")
	b.WriteString("mkdir -p docs/archive/{development,testing,meetings,investigations}\n")
	b.WriteString("```\n\n")

	b.WriteString("## Step 5: Move Files\n\n")
	b.WriteString("For each file:\n")
	b.WriteString("1. Determine correct location\n")
	b.WriteString("2. Rename according to standards\n")
	b.WriteString("3. Move to correct directory\n\n")

	b.WriteString("**Example commands:**\n")
	b.WriteString("```bash\n")
	b.WriteString("# Move active documentation\n")
	b.WriteString("mv CONFIG_VALIDATION.md docs/guides/configuration-validation.md\n\n")
	b.WriteString("# Archive development sessions\n")
	b.WriteString("mv BADGER_VALIDATION.md docs/archive/development/2025-10-27-badger-validation.md\n")
	b.WriteString("```\n\n")

	if createIndex == "true" {
		b.WriteString("## Step 6: Create/Update Documentation Index\n\n")
		b.WriteString("Create or update `docs/index.md` with:\n")
		b.WriteString("- Links to all documentation organized by topic\n")
		b.WriteString("- Brief descriptions\n")
		b.WriteString("- Documentation guidelines section\n")
		b.WriteString("- Filename rules\n\n")

		b.WriteString("**Index structure:**\n")
		b.WriteString("```markdown\n")
		b.WriteString("# Documentation Index\n\n")
		b.WriteString("## Guides\n")
		b.WriteString("- [Configuration Validation](guides/configuration-validation.md)\n\n")
		b.WriteString("## Archive\n")
		b.WriteString("### Development Sessions\n")
		b.WriteString("- [2025-10-27: BadgerDB Validation](archive/development/2025-10-27-badger-validation.md)\n")
		b.WriteString("```\n\n")
	}

	b.WriteString("## Step 7: Verify Organization\n\n")
	b.WriteString("**Checklist:**\n")
	b.WriteString("- [ ] No markdown files in root (except standard files)\n")
	b.WriteString("- [ ] All documentation in docs/ or subdirectories\n")
	b.WriteString("- [ ] Filenames follow standards\n")
	b.WriteString("- [ ] Index is up to date\n")
	b.WriteString("- [ ] No broken links\n")
	b.WriteString("- [ ] Archive files have dates\n\n")

	b.WriteString("## Step 8: Commit Changes\n\n")
	b.WriteString("```bash\n")
	b.WriteString("git add docs/\n")
	b.WriteString("git commit -m \"Organize project documentation\"\n")
	b.WriteString("git push\n")
	b.WriteString("```\n\n")

	b.WriteString("## Documentation Guidelines Reference\n\n")
	b.WriteString("**When to Archive:**\n")
	b.WriteString("- Development session notes\n")
	b.WriteString("- Test results\n")
	b.WriteString("- Historical information\n\n")

	b.WriteString("**When to Delete:**\n")
	b.WriteString("- Information integrated elsewhere\n")
	b.WriteString("- Completely obsolete content\n")
	b.WriteString("- Duplicates\n\n")

	b.WriteString("**Filename Standards:**\n")
	b.WriteString("- lowercase-with-hyphens.md\n")
	b.WriteString("- YYYY-MM-DD-description.md (for archives)\n")
	b.WriteString("- Descriptive, clear names\n\n")

	return b.String()
}

func generatePrepareMainnetFollowerTemplate(args map[string]string, getArg func(string, string) string) string {
	workDir := args["work_dir"]
	bvn := getArg("bvn", "Cyclops")
	dnDatabase := getArg("dn_database", "")
	bvnDatabase := getArg("bvn_database", "")

	var b strings.Builder
	b.WriteString("# Prepare Mainnet Follower Deployment\n\n")
	b.WriteString(fmt.Sprintf("**Target Directory:** `%s`\n", workDir))
	b.WriteString(fmt.Sprintf("**BVN Partition:** %s\n\n", bvn))

	b.WriteString("---\n\n")

	b.WriteString("## Phase 1: System Prerequisites\n\n")
	b.WriteString("Run `accumulate_validate_prerequisites` to verify system requirements:\n\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s",`+"\n", workDir))
	b.WriteString(`  "network": "mainnet"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("### Required Checks\n\n")
	b.WriteString("| Requirement | Minimum | Recommended | Check |\n")
	b.WriteString("|-------------|---------|-------------|-------|\n")
	b.WriteString("| Disk Space | 50 GB | 100+ GB | `df -h` |\n")
	b.WriteString("| Memory | 4 GB | 8+ GB | `free -h` |\n")
	b.WriteString("| CPU Cores | 2 | 4+ | `nproc` |\n")
	b.WriteString("| Docker | Installed | Running | `docker info` |\n")
	b.WriteString("| Ports | 16591-16593, 16691-16693 | Available | Tool checks |\n")
	b.WriteString("| Network | Bootstrap reachable | Low latency | Tool checks |\n\n")

	b.WriteString("### If Prerequisites Fail\n\n")
	b.WriteString("**Disk space insufficient:**\n")
	b.WriteString("- Free up space: `sudo apt clean`, remove old logs\n")
	b.WriteString("- Use a different directory on a larger volume\n\n")

	b.WriteString("**Docker not running:**\n")
	b.WriteString("```bash\n")
	b.WriteString("sudo systemctl start docker\n")
	b.WriteString("sudo systemctl enable docker\n")
	b.WriteString("```\n\n")

	b.WriteString("**Ports in use:**\n")
	b.WriteString("```bash\n")
	b.WriteString("sudo lsof -i :16591  # Find what's using the port\n")
	b.WriteString("```\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Phase 2: Network Connectivity\n\n")
	b.WriteString("Query the bootstrap server to verify network access:\n\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_query_bootstrap_server {}\n")
	b.WriteString("```\n\n")

	b.WriteString("### Expected Response\n")
	b.WriteString("- `overall_status`: `healthy`\n")
	b.WriteString("- `health.peer_count`: > 0\n")
	b.WriteString("- `health.conn_count`: > 0\n\n")

	b.WriteString("### If Bootstrap Unreachable\n")
	b.WriteString("1. Check firewall allows outbound connections\n")
	b.WriteString("2. Verify DNS resolution: `nslookup bootstrap.accumulate.defidevs.io`\n")
	b.WriteString("3. Test connectivity: `curl http://bootstrap.accumulate.defidevs.io:8080/health`\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Phase 3: Database Snapshots\n\n")

	if dnDatabase != "" && bvnDatabase != "" {
		b.WriteString("**Snapshots Provided:**\n")
		b.WriteString(fmt.Sprintf("- DN: `%s`\n", dnDatabase))
		b.WriteString(fmt.Sprintf("- BVN: `%s`\n\n", bvnDatabase))

		b.WriteString("### Verify Snapshot Integrity\n\n")
		b.WriteString("Check that snapshots are valid:\n")
		b.WriteString("```bash\n")
		b.WriteString(fmt.Sprintf("ls -la %s/data/accumulate.db/\n", dnDatabase))
		b.WriteString(fmt.Sprintf("ls -la %s/data/accumulate.db/\n", bvnDatabase))
		b.WriteString("```\n\n")
	} else {
		b.WriteString("### Snapshot Sources\n\n")
		b.WriteString("Database snapshots can be obtained from:\n\n")
		b.WriteString("1. **Existing node backup** - Copy from a running node\n")
		b.WriteString("2. **Network archive** - Download from snapshot archive service\n")
		b.WriteString("3. **Fresh sync** - Start from genesis (takes longest)\n\n")

		b.WriteString("### Snapshot Requirements\n\n")
		b.WriteString("| Component | Description | Approximate Size |\n")
		b.WriteString("|-----------|-------------|------------------|\n")
		b.WriteString("| DN Database | Directory Network state | ~5-10 GB |\n")
		b.WriteString(fmt.Sprintf("| %s Database | BVN partition state | ~10-20 GB |\n\n", bvn))

		b.WriteString("### Snapshot Age Considerations\n\n")
		b.WriteString("| Age | Sync Time | Recommendation |\n")
		b.WriteString("|-----|-----------|----------------|\n")
		b.WriteString("| < 1 day | Minutes | Excellent |\n")
		b.WriteString("| 1-7 days | Hours | Good |\n")
		b.WriteString("| 1-4 weeks | 12-24 hours | Acceptable |\n")
		b.WriteString("| > 1 month | Days | Consider fresher snapshot |\n\n")
	}

	b.WriteString("---\n\n")

	b.WriteString("## Phase 4: Ready for Deployment\n\n")
	b.WriteString("Once all prerequisites pass, proceed with deployment:\n\n")
	b.WriteString("```\n")
	b.WriteString("Use prompt: deploy-follower-node\n")
	b.WriteString("  dn_database: [path to DN snapshot]\n")
	b.WriteString("  bvn_database: [path to BVN snapshot]\n")
	b.WriteString(fmt.Sprintf("  work_dir: %s\n", workDir))
	b.WriteString("```\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Preparation Checklist\n\n")
	b.WriteString("- [ ] Prerequisites validated (all checks pass)\n")
	b.WriteString("- [ ] Bootstrap server reachable\n")
	b.WriteString("- [ ] DN database snapshot available\n")
	b.WriteString(fmt.Sprintf("- [ ] %s database snapshot available\n", bvn))
	b.WriteString("- [ ] Snapshot integrity verified\n")
	b.WriteString(fmt.Sprintf("- [ ] Work directory ready: `%s`\n\n", workDir))

	b.WriteString("## Next Steps\n\n")
	b.WriteString("- `deploy-follower-node` - Full deployment workflow\n")
	b.WriteString("- `monitor-follower-health` - Post-deployment monitoring\n")

	return b.String()
}

func generateRecoveryFromFailureTemplate(args map[string]string, getArg func(string, string) string) string {
	containerName := getArg("container_name", "accumulate-follower")
	failureType := getArg("failure_type", "unknown")
	workDir := getArg("work_dir", "")

	var b strings.Builder
	b.WriteString("# Follower Recovery Guide\n\n")
	b.WriteString(fmt.Sprintf("**Container:** `%s`\n", containerName))
	if failureType != "unknown" {
		b.WriteString(fmt.Sprintf("**Reported Failure:** %s\n", failureType))
	}
	b.WriteString("\n---\n\n")

	b.WriteString("## Step 1: Diagnose the Issue\n\n")

	b.WriteString("### Check Container Status\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_follower_status {\n")
	b.WriteString(fmt.Sprintf(`  "container_name": "%s"`+"\n", containerName))
	b.WriteString("}\n```\n\n")

	b.WriteString("### Analyze Logs\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_analyze_logs {\n")
	b.WriteString(fmt.Sprintf(`  "container_name": "%s",`+"\n", containerName))
	b.WriteString(`  "lines": 1000,`+"\n")
	b.WriteString(`  "filter": "all"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("### Check Sync Progress\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_get_sync_progress {\n")
	b.WriteString(fmt.Sprintf(`  "container_name": "%s"`+"\n", containerName))
	b.WriteString("}\n```\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Step 2: Identify Failure Type\n\n")
	b.WriteString("Based on diagnostics, identify which failure type matches:\n\n")

	b.WriteString("### Type A: Container Crashed\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- Container status: `stopped` or `not_found`\n")
	b.WriteString("- Log analysis shows: `panic`, `fatal`, or `critical` errors\n\n")

	b.WriteString("**Recovery Steps:**\n")
	b.WriteString("1. Review the crash logs for root cause\n")
	b.WriteString("2. If OOM (out of memory):\n")
	b.WriteString("   - Increase system memory or add swap\n")
	b.WriteString("   - Reduce other workloads\n")
	b.WriteString("3. If disk full:\n")
	b.WriteString("   - Free up disk space\n")
	if workDir != "" {
		b.WriteString(fmt.Sprintf("   - Check: `df -h %s`\n", workDir))
	}
	b.WriteString("4. Restart the container:\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_run_follower {\n")
	if workDir != "" {
		b.WriteString(fmt.Sprintf(`     "work_dir": "%s"`+"\n", workDir))
	} else {
		b.WriteString(`     "work_dir": "/path/to/work_dir"`+"\n")
	}
	b.WriteString("   }\n")
	b.WriteString("   ```\n\n")

	b.WriteString("### Type B: Sync Stalled\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- Container status: `running`\n")
	b.WriteString("- Sync progress: `stalled` or blocks_behind not decreasing\n")
	b.WriteString("- Peer count: 0 or very low\n\n")

	b.WriteString("**Recovery Steps:**\n")
	b.WriteString("1. Check network connectivity:\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_query_bootstrap_server {}\n")
	b.WriteString("   ```\n")
	b.WriteString("2. Verify firewall allows ports 16591-16593, 16691-16693\n")
	b.WriteString("3. Restart the follower:\n")
	b.WriteString("   ```json\n")
	b.WriteString(fmt.Sprintf("   accumulate_stop_follower { \"container_name\": \"%s\" }\n", containerName))
	b.WriteString("   // Wait 10 seconds\n")
	b.WriteString("   accumulate_run_follower { ... }\n")
	b.WriteString("   ```\n")
	b.WriteString("4. If persists, re-initialize with fresh peers:\n")
	b.WriteString("   - Use `accumulate_init_follower` with `auto_discover_peers: true`\n\n")

	b.WriteString("### Type C: No Peers\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- Peer count: 0\n")
	b.WriteString("- Log shows connection failures\n\n")

	b.WriteString("**Recovery Steps:**\n")
	b.WriteString("1. Verify bootstrap server is healthy\n")
	b.WriteString("2. Check firewall configuration\n")
	b.WriteString("3. Test port connectivity:\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   nc -zv bootstrap.accumulate.defidevs.io 16593\n")
	b.WriteString("   ```\n")
	b.WriteString("4. Re-initialize with updated peers\n\n")

	b.WriteString("### Type D: Database Corruption\n")
	b.WriteString("**Symptoms:**\n")
	b.WriteString("- Log shows database errors\n")
	b.WriteString("- Repeated crashes on startup\n")
	b.WriteString("- Badger errors in logs\n\n")

	b.WriteString("**Recovery Steps:**\n")
	b.WriteString("1. Stop the container\n")
	b.WriteString("2. **Backup current state** (if any data is valuable)\n")
	b.WriteString("3. Re-deploy from fresh snapshots:\n")
	b.WriteString("   - Remove old data\n")
	b.WriteString("   - Run `accumulate_init_follower` with fresh snapshots\n")
	b.WriteString("   - Start follower\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Step 3: Verify Recovery\n\n")
	b.WriteString("After applying recovery steps:\n\n")

	b.WriteString("1. **Check container is running:**\n")
	b.WriteString("   ```json\n")
	b.WriteString(fmt.Sprintf("   accumulate_follower_status { \"container_name\": \"%s\" }\n", containerName))
	b.WriteString("   ```\n\n")

	b.WriteString("2. **Verify sync is progressing:**\n")
	b.WriteString("   ```json\n")
	b.WriteString("   accumulate_get_sync_progress { \"include_rate\": true }\n")
	b.WriteString("   ```\n\n")

	b.WriteString("3. **Monitor for 5-10 minutes:**\n")
	b.WriteString("   - Sync percentage should increase\n")
	b.WriteString("   - Peer count should be 3+\n")
	b.WriteString("   - No new critical errors\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## When to Re-Deploy\n\n")
	b.WriteString("Consider full re-deployment if:\n")
	b.WriteString("- Database corruption confirmed\n")
	b.WriteString("- Multiple recovery attempts failed\n")
	b.WriteString("- Snapshots are very old (> 1 month)\n")
	b.WriteString("- Configuration is completely broken\n\n")

	b.WriteString("**Re-deployment steps:**\n")
	b.WriteString("1. `accumulate_remove_follower` - Remove old container\n")
	b.WriteString("2. Delete old work directory contents\n")
	b.WriteString("3. `prepare-mainnet-follower` - Prepare fresh deployment\n")
	b.WriteString("4. `deploy-follower-node` - Deploy from scratch\n\n")

	b.WriteString("## Related Prompts\n\n")
	b.WriteString("- `troubleshoot-follower-sync` - Detailed sync troubleshooting\n")
	b.WriteString("- `monitor-follower-health` - Ongoing monitoring\n")
	b.WriteString("- `deploy-follower-node` - Fresh deployment\n")

	return b.String()
}

func generateMainnetSyncStatusTemplate(args map[string]string, getArg func(string, string) string) string {
	containerName := getArg("container_name", "accumulate-follower")

	var b strings.Builder
	b.WriteString("# Mainnet Sync Status\n\n")

	b.WriteString("## Quick Status Check\n\n")
	b.WriteString("Run these tools in sequence for a complete status view:\n\n")

	b.WriteString("### 1. Container Status\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_follower_status {\n")
	b.WriteString(fmt.Sprintf(`  "container_name": "%s"`+"\n", containerName))
	b.WriteString("}\n```\n\n")

	b.WriteString("### 2. Sync Progress\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_get_sync_progress {\n")
	b.WriteString(fmt.Sprintf(`  "container_name": "%s",`+"\n", containerName))
	b.WriteString(`  "include_rate": true`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("### 3. Network Comparison\n")
	b.WriteString("```json\n")
	b.WriteString("accumulate_network_status {\n")
	b.WriteString(`  "network": "mainnet"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Status Interpretation\n\n")

	b.WriteString("### Sync Status Values\n\n")
	b.WriteString("| Status | Meaning | Action |\n")
	b.WriteString("|--------|---------|--------|\n")
	b.WriteString("| `synced` | Fully caught up | Monitor periodically |\n")
	b.WriteString("| `syncing` | Catching up to network | Wait, monitor progress |\n")
	b.WriteString("| `stalled` | Not making progress | Investigate (see below) |\n")
	b.WriteString("| `stopped` | Container not running | Restart follower |\n")
	b.WriteString("| `not_found` | Container missing | Re-deploy |\n\n")

	b.WriteString("### Health Indicators\n\n")
	b.WriteString("| Metric | Healthy | Warning | Critical |\n")
	b.WriteString("|--------|---------|---------|----------|\n")
	b.WriteString("| Blocks Behind | < 100 | 100-1000 | > 1000 |\n")
	b.WriteString("| Peer Count | 3+ | 1-2 | 0 |\n")
	b.WriteString("| Sync Rate | > 50/min | 10-50/min | < 10/min |\n")
	b.WriteString("| Sync % | > 99% | 90-99% | < 90% |\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Quick Reference Output\n\n")
	b.WriteString("Expected output format from `accumulate_get_sync_progress`:\n\n")
	b.WriteString("```\n")
	b.WriteString("{\n")
	b.WriteString("  \"status\": \"syncing\",\n")
	b.WriteString("  \"local_height\": 15234123,\n")
	b.WriteString("  \"network_height\": 15234567,\n")
	b.WriteString("  \"blocks_behind\": 444,\n")
	b.WriteString("  \"sync_percentage\": 99.97,\n")
	b.WriteString("  \"estimated_eta\": \"3 minutes\",\n")
	b.WriteString("  \"sync_rate_blocks\": 150.0,\n")
	b.WriteString("  \"peer_count\": 5,\n")
	b.WriteString("  \"container_status\": \"running\"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## If Issues Detected\n\n")
	b.WriteString("| Issue | Prompt to Use |\n")
	b.WriteString("|-------|---------------|\n")
	b.WriteString("| Sync stalled | `troubleshoot-follower-sync` |\n")
	b.WriteString("| Container stopped | `recovery-from-failure` |\n")
	b.WriteString("| No peers | `troubleshoot-follower-sync` |\n")
	b.WriteString("| Falling behind | `monitor-follower-health` |\n\n")

	b.WriteString("## Related Prompts\n\n")
	b.WriteString("- `monitor-follower-health` - Detailed health monitoring\n")
	b.WriteString("- `quick-node-status` - Even faster check\n")
	b.WriteString("- `troubleshoot-follower-sync` - If issues found\n")

	return b.String()
}

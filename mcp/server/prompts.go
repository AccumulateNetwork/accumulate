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
	b.WriteString("Monitor Accumulate follower health\n\n")
	b.WriteString("**Quick Health Check:**\n\n")

	b.WriteString("Step 1: Check process status\n")
	b.WriteString("Use `accumulate_follower_status`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "work_dir": "%s"`+"\n", workDir))
	b.WriteString("}\n```\n\n")

	b.WriteString("**Process Health:**\n")
	b.WriteString("- [ ] Running: YES/NO\n")
	b.WriteString("- [ ] PID: [number]\n")
	b.WriteString("- [ ] Uptime: [duration]\n\n")

	b.WriteString("Step 2: Get node information\n")
	b.WriteString("Use `accumulate_node_info`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(fmt.Sprintf(`  "network": "%s"`+"\n", endpoint))
	b.WriteString("}\n```\n\n")

	b.WriteString("**Node Metrics:**\n")
	b.WriteString("- Current Block: [height]\n")
	b.WriteString("- Peer Count: [count]\n")
	b.WriteString("- Sync Status: [syncing/synced/behind]\n")
	b.WriteString("- Last Block Time: [timestamp]\n\n")

	b.WriteString("Step 3: Get network status (for comparison)\n")
	b.WriteString("Use `accumulate_network_status`:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString(`  "network": "mainnet"`+"\n")
	b.WriteString("}\n```\n\n")

	b.WriteString("**Network Comparison:**\n")
	b.WriteString("- Network Height: [height]\n")
	b.WriteString("- Follower Height: [height from step 2]\n")
	b.WriteString("- Blocks Behind: [difference]\n")
	b.WriteString("- Catch-up Rate: [blocks/minute if syncing]\n\n")

	b.WriteString("**Health Status Summary:**\n\n")
	b.WriteString("✅ **HEALTHY** if:\n")
	b.WriteString("- Process running\n")
	b.WriteString("- Peers ≥ 3\n")
	b.WriteString("- Blocks behind < 100 OR syncing actively\n")
	b.WriteString("- No critical errors in logs\n\n")

	b.WriteString("⚠️ **WARNING** if:\n")
	b.WriteString("- Peers < 3 but > 0\n")
	b.WriteString("- Blocks behind 100-1000\n")
	b.WriteString("- Slow catch-up rate\n\n")

	b.WriteString("❌ **UNHEALTHY** if:\n")
	b.WriteString("- Process not running\n")
	b.WriteString("- Peers = 0\n")
	b.WriteString("- Blocks behind > 1000 and not catching up\n")
	b.WriteString("- Critical errors in logs\n\n")

	b.WriteString("**Recommended Actions:**\n\n")
	b.WriteString("If HEALTHY:\n")
	b.WriteString("  ✅ Continue monitoring\n")
	b.WriteString("  ✅ Check again in 15-30 minutes\n\n")

	b.WriteString("If WARNING:\n")
	b.WriteString("  ⚠️ Investigate peer connections\n")
	b.WriteString("  ⚠️ Check network connectivity\n")
	b.WriteString("  ⚠️ Monitor for 10-15 minutes\n")
	b.WriteString("  ⚠️ If persists, use `troubleshoot-follower-sync`\n\n")

	b.WriteString("If UNHEALTHY:\n")
	b.WriteString("  ❌ Use `troubleshoot-follower-sync` prompt immediately\n")
	b.WriteString("  ❌ Review recent logs\n")
	b.WriteString("  ❌ Consider restart if stuck\n\n")

	b.WriteString("**Related Prompts:**\n")
	b.WriteString("- `troubleshoot-follower-sync` - If issues detected\n")
	b.WriteString("- `deploy-follower-node` - Initial deployment\n")
	b.WriteString("- `quick-node-status` - Even faster check\n")

	return b.String()
}

func generateTroubleshootFollowerSyncTemplate(args map[string]string, getArg func(string, string) string) string {
	workDir := getArg("work_dir", "~/.accumulate/follower")
	symptom := getArg("symptom", "general")

	var b strings.Builder
	b.WriteString("Troubleshoot Accumulate Follower Sync Issues\n")
	b.WriteString(fmt.Sprintf("Symptom: %s\n\n", symptom))

	b.WriteString("**Diagnostic Steps:**\n\n")

	b.WriteString("**1. Process Check**\n")
	b.WriteString("Use `accumulate_follower_status`\n")
	b.WriteString("- Is process running? YES/NO\n")
	b.WriteString("- If NO → Process crashed or not started\n")
	b.WriteString("- If YES → Continue diagnostics\n\n")

	b.WriteString("**2. Peer Connection Check**\n")
	b.WriteString("Use `accumulate_node_info`\n")
	b.WriteString("- Peer count: [number]\n")
	b.WriteString("- If 0 peers → Network connectivity issue\n")
	b.WriteString("- If 1-2 peers → Degraded but may work\n")
	b.WriteString("- If 3+ peers → Peers OK\n\n")

	b.WriteString("**3. Block Height Check**\n")
	b.WriteString("Use `accumulate_node_info` + `accumulate_network_status`\n")
	b.WriteString("- Local height: [number]\n")
	b.WriteString("- Network height: [number]\n")
	b.WriteString("- Behind by: [difference]\n")
	b.WriteString("- If not advancing → Sync stalled\n\n")

	b.WriteString("**4. Log Review**\n")
	b.WriteString("Check recent logs for:\n")
	b.WriteString("- Database errors\n")
	b.WriteString("- Network errors\n")
	b.WriteString("- Consensus errors\n")
	b.WriteString("- Panic/crash messages\n\n")

	b.WriteString("**Issue-Specific Troubleshooting:**\n\n")

	b.WriteString("### SYMPTOM: no_peers (Peer count = 0)\n\n")
	b.WriteString("**Likely Causes:**\n")
	b.WriteString("1. Firewall blocking ports\n")
	b.WriteString("2. Incorrect peer URL\n")
	b.WriteString("3. Network connectivity\n")
	b.WriteString("4. Peer is down\n\n")

	b.WriteString("**Resolution Steps:**\n\n")
	b.WriteString("A. Verify network connectivity\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   # Check if peer URL is reachable\n")
	b.WriteString("   telnet mainnet.accumulate.defidevs.io 16691\n")
	b.WriteString("   ```\n")
	b.WriteString("   If fails → Network/firewall issue\n\n")

	b.WriteString("B. Check firewall rules\n")
	b.WriteString("   - Ports 16591-16593 must be open\n")
	b.WriteString("   - Both inbound and outbound\n")
	b.WriteString("   - Check: `sudo ufw status` or `iptables -L`\n\n")

	b.WriteString("C. Try alternative peer\n")
	b.WriteString("   - Use `accumulate_init_follower` with different peer_url\n")
	b.WriteString("   - Mainnet peers:\n")
	b.WriteString("     - tcp://mainnet.accumulate.defidevs.io:16691\n\n")

	b.WriteString("D. Verify configuration\n")
	b.WriteString(fmt.Sprintf("   - Check %s/accumulated.toml\n", workDir))
	b.WriteString("   - Verify peer settings correct\n\n")

	b.WriteString("**Fix:**\n")
	b.WriteString("If firewall issue:\n")
	b.WriteString("  ```bash\n")
	b.WriteString("  sudo ufw allow 16591:16593/tcp\n")
	b.WriteString("  ```\n\n")

	b.WriteString("If bad peer:\n")
	b.WriteString("  - Re-run init with good peer URL\n")
	b.WriteString("  - Use deploy-follower-node prompt with new peer\n\n")

	b.WriteString("---\n\n")

	b.WriteString("### SYMPTOM: not_syncing (Has peers but blocks not advancing)\n\n")
	b.WriteString("**Likely Causes:**\n")
	b.WriteString("1. Database corruption\n")
	b.WriteString("2. Old/incompatible snapshot\n")
	b.WriteString("3. Configuration mismatch\n")
	b.WriteString("4. Disk space full\n\n")

	b.WriteString("**Resolution Steps:**\n\n")
	b.WriteString("A. Check disk space\n")
	b.WriteString("   ```bash\n")
	b.WriteString(fmt.Sprintf("   df -h %s\n", workDir))
	b.WriteString("   ```\n")
	b.WriteString("   If <10% free → Disk full\n\n")

	b.WriteString("B. Check database health\n")
	b.WriteString("   - Look for \"database\" errors in logs\n")
	b.WriteString(fmt.Sprintf("   - Check %s/dnn and /bvnn intact\n\n", workDir))

	b.WriteString("C. Verify snapshot compatibility\n")
	b.WriteString("   - Snapshots should be < 1 month old\n")
	b.WriteString("   - Must match network (mainnet vs testnet)\n\n")

	b.WriteString("D. Check configuration\n")
	b.WriteString("   - Review accumulated.toml\n")
	b.WriteString("   - Verify network settings match\n\n")

	b.WriteString("**Fix:**\n")
	b.WriteString("If disk full:\n")
	b.WriteString("  - Free up space\n")
	b.WriteString("  - Consider larger volume\n\n")

	b.WriteString("If database issue:\n")
	b.WriteString("  - May need to re-deploy with fresh snapshots\n")
	b.WriteString("  - Use deploy-follower-node with recent snapshots\n\n")

	b.WriteString("If config issue:\n")
	b.WriteString("  - Restore from backup or re-init\n\n")

	b.WriteString("---\n\n")

	b.WriteString("### SYMPTOM: slow_sync (Syncing but very slow)\n\n")
	b.WriteString("**Likely Causes:**\n")
	b.WriteString("1. Limited peers (1-2 instead of 3+)\n")
	b.WriteString("2. Slow storage (HDD vs SSD)\n")
	b.WriteString("3. Network bandwidth limited\n")
	b.WriteString("4. CPU/memory constrained\n\n")

	b.WriteString("**Resolution Steps:**\n\n")
	b.WriteString("A. Check resources\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   htop  # Check CPU/memory\n")
	b.WriteString("   iostat -x 1  # Check disk I/O\n")
	b.WriteString("   ```\n\n")

	b.WriteString("B. Verify peer count\n")
	b.WriteString("   - Need 3+ peers for optimal sync\n")
	b.WriteString("   - If <3, may need better peer URLs\n\n")

	b.WriteString("C. Check network bandwidth\n")
	b.WriteString("   - Syncing requires sustained download\n")
	b.WriteString("   - Monitor with `iftop` or similar\n\n")

	b.WriteString("**Fix:**\n")
	b.WriteString("If resource constrained:\n")
	b.WriteString("  - Upgrade to SSD storage\n")
	b.WriteString("  - Increase memory allocation\n")
	b.WriteString("  - Use less loaded system\n\n")

	b.WriteString("If peer limited:\n")
	b.WriteString("  - Add more peer URLs to config\n")
	b.WriteString("  - Ensure ports not rate-limited\n\n")

	b.WriteString("---\n\n")

	b.WriteString("### SYMPTOM: crashed (Process died)\n\n")
	b.WriteString("**Likely Causes:**\n")
	b.WriteString("1. Out of memory\n")
	b.WriteString("2. Database corruption\n")
	b.WriteString("3. Bug/panic in code\n")
	b.WriteString("4. Disk full\n\n")

	b.WriteString("**Resolution Steps:**\n\n")
	b.WriteString("A. Check crash logs\n")
	b.WriteString("   - Review accumulated.log\n")
	b.WriteString("   - Look for \"panic\" or \"fatal\"\n")
	b.WriteString("   - Note exact error message\n\n")

	b.WriteString("B. Check system resources\n")
	b.WriteString("   ```bash\n")
	b.WriteString("   dmesg | grep -i killed  # OOM killer?\n")
	b.WriteString("   df -h  # Disk space?\n")
	b.WriteString("   free -h  # Memory available?\n")
	b.WriteString("   ```\n\n")

	b.WriteString("C. Try restart\n")
	b.WriteString("   - Use `accumulate_run_follower`\n")
	b.WriteString("   - Monitor if crashes again\n\n")

	b.WriteString("**Fix:**\n")
	b.WriteString("If OOM:\n")
	b.WriteString("  - Increase system memory\n")
	b.WriteString("  - Add swap space\n")
	b.WriteString("  - Reduce other processes\n\n")

	b.WriteString("If database corruption:\n")
	b.WriteString("  - Re-deploy with fresh snapshots\n\n")

	b.WriteString("If persistent crash:\n")
	b.WriteString("  - Report bug with logs\n")
	b.WriteString("  - Try older/newer binary version\n\n")

	b.WriteString("---\n\n")

	b.WriteString("**General Recovery Procedure:**\n\n")
	b.WriteString("1. Stop follower\n")
	b.WriteString("2. Backup current state\n")
	b.WriteString("3. Review all diagnostics above\n")
	b.WriteString("4. Apply specific fix\n")
	b.WriteString("5. Restart follower\n")
	b.WriteString("6. Monitor for 10-15 minutes\n")
	b.WriteString("7. If still failing → escalate or re-deploy\n\n")

	b.WriteString("**When to Re-deploy:**\n\n")
	b.WriteString("Consider fresh deployment if:\n")
	b.WriteString("- Database corruption confirmed\n")
	b.WriteString("- Snapshots > 1 month old\n")
	b.WriteString("- Configuration completely broken\n")
	b.WriteString("- Multiple fixes attempted without success\n\n")

	b.WriteString("Use `deploy-follower-node` with fresh snapshots\n\n")

	b.WriteString("**Prevention:**\n\n")
	b.WriteString("- Monitor regularly with `monitor-follower-health`\n")
	b.WriteString("- Keep snapshots recent (< 2 weeks)\n")
	b.WriteString("- Ensure adequate resources\n")
	b.WriteString("- Regular backups\n")
	b.WriteString("- Update to latest stable version\n\n")

	b.WriteString("**Related Prompts:**\n")
	b.WriteString("- `monitor-follower-health` - Regular monitoring\n")
	b.WriteString("- `deploy-follower-node` - Fresh deployment\n")
	b.WriteString("- `backup-follower` - Create backup before major changes\n")

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

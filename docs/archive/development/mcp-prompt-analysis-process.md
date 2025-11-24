# MCP Prompt Analysis Process

This document describes the repeatable process for analyzing a repository with an existing MCP server to identify, design, test, and implement useful prompts.

## Purpose

Prompts combine multiple MCP tools into workflow-oriented templates that make common tasks easier. This process helps identify what prompts would be most valuable for a given repository.

## Process Overview

```
1. Repository Analysis (Discovery)
2. Workflow Identification
3. Prompt Design
4. Implementation
5. Testing & Validation
6. Documentation
```

---

## Step 1: Repository Analysis (Discovery)

**Goal**: Understand what the repository does and what MCP capabilities it has.

### 1.1 Review Repository Documentation

Read in order:
- [ ] `README.md` - Understand repository purpose
- [ ] `CLAUDE.md` (if exists) - Development guidelines and workflows
- [ ] MCP design documents (`MCP_*.md`) - Understanding MCP architecture
- [ ] `active.md` - Current work and use cases
- [ ] `work-history.md` - Past patterns and decisions

**Questions to answer**:
- What is this repository for? (e.g., "staking rewards", "node deployment", "API server")
- Who are the users? (developers, operators, end-users)
- What are the main workflows? (build, deploy, query, manage)

**Example (staking repository)**:
```
Purpose: Staking rewards calculation and database management
Users: Developers querying rewards, operators managing databases
Workflows: Calculate rewards, query accounts, build databases, generate reports
```

### 1.2 Inventory Existing MCP Capabilities

Scan for:
- [ ] **Tools**: List all MCP tools with `ListTools()` or read MCP design docs
- [ ] **Resources**: Identify available resources (URIs)
- [ ] **Data**: What data can be accessed?

Create an inventory table:

| Type | Name | Purpose | Used For |
|------|------|---------|----------|
| Tool | `calculate-rewards` | Calculate staking rewards | Reward queries |
| Tool | `get-account-info` | Get account details | Account lookup |
| Resource | `staking://accounts` | All accounts | Batch processing |
| Resource | `staking://parameters` | Staking params | Configuration |

### 1.3 Identify Common Questions/Tasks

Look for:
- [ ] Issues filed (common problems)
- [ ] Active work items (what developers do frequently)
- [ ] Documentation examples (what's being explained often)
- [ ] Support requests (what users ask about)

**Document findings**:
```markdown
Common Tasks:
- "How do I calculate rewards for account X?"
- "What are the staking parameters for period Y?"
- "How do I deploy a follower node?"
- "How do I check if my node is syncing?"
```

---

## Step 2: Workflow Identification

**Goal**: Map user goals to multi-step workflows that prompts can simplify.

### 2.1 Categorize Workflows

Group by user intent:

**Development Workflows**:
- Setting up development environment
- Running tests
- Debugging issues
- Contributing code

**Deployment Workflows**:
- Initial deployment
- Configuration
- Monitoring
- Troubleshooting

**Operational Workflows**:
- Querying data
- Generating reports
- Monitoring health
- Performing maintenance

**Example for Follower Deployment**:
```
Deployment Workflow:
1. Check prerequisites (hardware, network)
2. Download/build binaries
3. Generate configuration
4. Initialize database
5. Start node
6. Verify synchronization
7. Monitor health
```

### 2.2 Map Tools to Workflows

For each workflow, identify which tools are needed:

**Example**:
```
Workflow: Deploy New Follower Node
├─ Step 1: Check prerequisites
│  └─ Tools: get-system-info, check-network-connectivity
├─ Step 2: Download binaries
│  └─ Tools: get-latest-release, download-binary
├─ Step 3: Generate config
│  └─ Tools: generate-config, get-bootstrap-peers
├─ Step 4: Initialize database
│  └─ Tools: init-database, verify-database
├─ Step 5: Start node
│  └─ Tools: start-node, check-process-status
└─ Step 6: Verify sync
   └─ Tools: get-sync-status, get-peer-count
```

### 2.3 Identify Prompt Opportunities

Good prompts should:
- Combine 2+ tools into a coherent workflow
- Reduce cognitive load (user doesn't need to remember tool sequence)
- Encode best practices
- Handle common variations

**Example Prompt Opportunities**:
```
✅ GOOD: "deploy-follower-node"
   - Combines 6+ tools
   - Walks through entire workflow
   - Checks prerequisites
   - Validates each step

❌ BAD: "get-account"
   - Just wraps one tool
   - Doesn't add value over calling tool directly
```

---

## Step 3: Prompt Design

**Goal**: Design effective prompt templates with clear structure.

### 3.1 Design Prompt Structure

For each identified prompt:

```go
{
    Name:        "prompt-name",
    Description: "One-line description of what this helps with",
    Arguments: []PromptArgument{
        {
            Name:        "arg_name",
            Description: "What this argument is for",
            Required:    true/false,
        },
    },
    Template: func(args map[string]string) string {
        return `[prompt template]`
    },
}
```

### 3.2 Write Prompt Templates

**Template Structure**:

```markdown
[Brief introduction explaining the task]

**Context:**
[Repository/feature being used: ${arg1}]

**Steps:**
1. [First action using tool X]
2. [Second action using tool Y]
3. [Validation step]

**Tool Usage:**
- Use `tool-name` with arguments...
- Use `another-tool` to verify...

**Expected Output:**
- [What success looks like]
- [What to check for]
- [Common issues and solutions]

**Validation:**
- Verify X happened
- Check Y is correct
- Confirm Z is working

**Next Steps:**
[What to do after this workflow completes]
```

### 3.3 Example: Follower Deployment Prompt

```go
{
    Name:        "deploy-follower-node",
    Description: "Complete workflow for deploying a new Accumulate follower node",
    Arguments: []PromptArgument{
        {Name: "network", Description: "Network to join (mainnet/testnet)", Required: true},
        {Name: "node_name", Description: "Name for this node", Required: true},
        {Name: "data_dir", Description: "Data directory path", Required: false},
    },
    Template: func(args map[string]string) string {
        dataDir := args["data_dir"]
        if dataDir == "" {
            dataDir = "~/.accumulate"
        }
        return `Deploy Accumulate follower node: ` + args["node_name"] + ` on ` + args["network"] + `

**Prerequisites Check:**
1. Use \`check-system-requirements\` to verify:
   - CPU: 4+ cores
   - RAM: 8+ GB
   - Disk: 100+ GB free at ` + dataDir + `
   - Network: Open ports 16591-16593

**Download and Setup:**
2. Use \`get-latest-release\` for network: ` + args["network"] + `
3. Use \`download-binary\` to fetch accumulated binary
4. Use \`verify-binary-checksum\` to ensure integrity

**Configuration:**
5. Use \`generate-node-config\` with:
   - network: ` + args["network"] + `
   - node-type: follower
   - node-name: ` + args["node_name"] + `
   - data-dir: ` + dataDir + `

6. Use \`get-bootstrap-peers\` for network: ` + args["network"] + `
7. Add bootstrap peers to config

**Database Initialization:**
8. Use \`init-database\` with data-dir: ` + dataDir + `
9. Use \`verify-database-structure\` to confirm

**Start Node:**
10. Use \`start-node\` with config path
11. Use \`check-process-status\` to verify running
12. Use \`tail-logs\` to monitor startup

**Synchronization:**
13. Use \`get-sync-status\` - should show "syncing"
14. Use \`get-peer-count\` - should have 3+ peers
15. Use \`get-current-block\` - should be increasing

**Validation Checklist:**
- [ ] Process running (check-process-status)
- [ ] Peers connected (get-peer-count ≥ 3)
- [ ] Syncing active (get-sync-status = "syncing")
- [ ] Blocks advancing (get-current-block increasing)
- [ ] No errors in logs (tail-logs)

**Expected Timeline:**
- Startup: 1-2 minutes
- First peers: 2-5 minutes
- Sync start: 5-10 minutes
- Full sync: 2-24 hours (depending on network)

**Troubleshooting:**
If peers = 0:
  - Use \`check-network-connectivity\` to verify ports
  - Use \`get-bootstrap-peers\` to refresh peer list

If sync not starting:
  - Use \`tail-logs\` with filter: "sync"
  - Use \`verify-database-structure\` to check DB

If errors in logs:
  - Use \`diagnose-common-errors\` with log output

**Next Steps:**
- Monitor sync progress: Use prompt "monitor-follower-sync"
- Set up monitoring: Use prompt "setup-node-monitoring"
- Configure backups: Use prompt "setup-node-backups"

**Reference:**
- Network: ` + args["network"] + `
- Node: ` + args["node_name"] + `
- Data: ` + dataDir + `
`
    },
}
```

### 3.4 Design Patterns

**Pattern 1: Step-by-Step Workflows**
```
Use for: Multi-step processes
Structure: Numbered steps with tool calls
Example: deploy-node, setup-environment
```

**Pattern 2: Validation & Health Checks**
```
Use for: Checking system state
Structure: Checklist with validation tools
Example: verify-node-health, check-deployment-status
```

**Pattern 3: Troubleshooting Guides**
```
Use for: Debugging problems
Structure: If/then branches with diagnostic tools
Example: diagnose-sync-issues, fix-configuration-errors
```

**Pattern 4: Quick Status**
```
Use for: Fast overviews
Structure: Brief tool calls, minimal output
Example: quick-node-status, deployment-summary
```

---

## Step 4: Implementation

**Goal**: Code the prompts into the MCP server.

### 4.1 Create Prompts File

```bash
# In repository's MCP server directory
touch prompts.go
```

### 4.2 Implement Prompt Definitions

```go
package main

// PromptDefinition defines an MCP prompt template
type PromptDefinition struct {
    Name        string
    Description string
    Arguments   []PromptArgument
    Template    func(args map[string]string) string
}

// PromptArgument defines a prompt argument
type PromptArgument struct {
    Name        string
    Description string
    Required    bool
}

// GetPrompts returns all available prompt templates
func GetPrompts() []PromptDefinition {
    return []PromptDefinition{
        // Add your prompts here
        {
            Name:        "prompt-name",
            Description: "Description",
            Arguments:   []PromptArgument{...},
            Template:    func(args map[string]string) string {...},
        },
    }
}
```

### 4.3 Integrate with Server

Update server to support prompts:

```go
// In server.go

// ListPrompts returns available prompts
func (s *Server) ListPrompts() []PromptDefinition {
    return GetPrompts()
}

// GetPrompt returns a specific prompt with args applied
func (s *Server) GetPrompt(name string, args map[string]string) (string, error) {
    prompts := GetPrompts()

    for _, prompt := range prompts {
        if prompt.Name == name {
            // Validate required arguments
            for _, arg := range prompt.Arguments {
                if arg.Required {
                    if _, ok := args[arg.Name]; !ok {
                        return "", fmt.Errorf("missing required argument: %s", arg.Name)
                    }
                }
            }

            // Apply template
            return prompt.Template(args), nil
        }
    }

    return "", fmt.Errorf("prompt not found: %s", name)
}
```

### 4.4 Register Prompts

```go
// In server initialization
func NewServer(cfg *Config) *Server {
    // ... existing code ...

    // Register prompts
    s.registerPrompts()

    return s
}

func (s *Server) registerPrompts() {
    s.logDebug("Prompt registration complete")
}
```

---

## Step 5: Testing & Validation

**Goal**: Ensure prompts work correctly and provide value.

### 5.1 Unit Tests

Test individual prompts:

```go
// prompts_test.go

func TestGetPrompts(t *testing.T) {
    prompts := GetPrompts()
    if len(prompts) == 0 {
        t.Fatal("No prompts defined")
    }

    for _, prompt := range prompts {
        if prompt.Name == "" {
            t.Error("Prompt has empty name")
        }
        if prompt.Description == "" {
            t.Errorf("Prompt %s has empty description", prompt.Name)
        }
        if prompt.Template == nil {
            t.Errorf("Prompt %s has nil template", prompt.Name)
        }
    }
}

func TestPromptTemplate(t *testing.T) {
    prompts := GetPrompts()

    // Test specific prompt
    var testPrompt *PromptDefinition
    for i := range prompts {
        if prompts[i].Name == "deploy-follower-node" {
            testPrompt = &prompts[i]
            break
        }
    }

    if testPrompt == nil {
        t.Fatal("Test prompt not found")
    }

    args := map[string]string{
        "network":   "mainnet",
        "node_name": "test-node",
    }

    result := testPrompt.Template(args)

    if !strings.Contains(result, "mainnet") {
        t.Error("Template didn't substitute network")
    }
    if !strings.Contains(result, "test-node") {
        t.Error("Template didn't substitute node_name")
    }
}

func TestServerGetPrompt(t *testing.T) {
    server := NewServer(&Config{})

    // Test valid prompt
    result, err := server.GetPrompt("deploy-follower-node", map[string]string{
        "network":   "testnet",
        "node_name": "my-node",
    })

    if err != nil {
        t.Fatalf("GetPrompt failed: %v", err)
    }

    if len(result) == 0 {
        t.Error("Prompt returned empty result")
    }

    // Test missing required arg
    _, err = server.GetPrompt("deploy-follower-node", map[string]string{
        "network": "testnet",
        // missing node_name
    })

    if err == nil {
        t.Error("Expected error for missing required argument")
    }
}
```

### 5.2 Integration Tests

Test with real repository data:

```go
func TestPromptsIntegration(t *testing.T) {
    // Use real paths
    cfg := &Config{
        DataDir: "/path/to/real/data",
    }
    server := NewServer(cfg)

    // Test each prompt category
    workflowPrompts := []string{
        "deploy-follower-node",
        "monitor-node-health",
    }

    for _, promptName := range workflowPrompts {
        // Test with valid args
        result, err := server.GetPrompt(promptName, map[string]string{
            "network":   "testnet",
            "node_name": "integration-test",
        })

        if err != nil {
            t.Errorf("Prompt %s failed: %v", promptName, err)
        }

        if len(result) < 100 {
            t.Errorf("Prompt %s output too short: %d chars", promptName, len(result))
        }
    }
}
```

### 5.3 Manual Validation

Test with MCP client (Claude Desktop):

1. Build and deploy MCP server
2. Configure Claude Desktop to use it
3. List available prompts
4. Execute each prompt with real arguments
5. Verify output is useful and actionable

**Validation Checklist**:
- [ ] Prompt appears in list
- [ ] Arguments are clear
- [ ] Template renders correctly
- [ ] Instructions are actionable
- [ ] Tool calls are correct
- [ ] Output helps accomplish task
- [ ] Error messages are helpful

---

## Step 6: Documentation

**Goal**: Document prompts for users and future reference.

### 6.1 Create Prompt Catalog

Create `prompts-summary.md`:

```markdown
# MCP Prompts - [Repository Name]

## Available Prompts

### Deployment Prompts

#### deploy-follower-node
**Purpose**: Deploy a new Accumulate follower node
**Arguments**:
- `network` (required): Network to join (mainnet/testnet)
- `node_name` (required): Name for this node
- `data_dir` (optional): Data directory path

**When to use**: When setting up a new follower node from scratch

**Example**:
```json
{
  "network": "mainnet",
  "node_name": "my-follower-1",
  "data_dir": "/data/accumulate"
}
```

**What it does**:
1. Checks system requirements
2. Downloads and verifies binary
3. Generates configuration
4. Initializes database
5. Starts node
6. Verifies synchronization

---

### Monitoring Prompts

[Additional prompts...]
```

### 6.2 Update Main Documentation

Add to README.md:

```markdown
## MCP Prompts

This server provides workflow-oriented prompts that combine multiple tools:

- **Deployment**: `deploy-follower-node`, `upgrade-node`
- **Monitoring**: `check-node-health`, `diagnose-sync-issues`
- **Operations**: `backup-node`, `restore-from-backup`

See [prompts-summary.md](./prompts-summary.md) for full catalog.
```

### 6.3 Add Examples

Create `examples/` directory with sample use cases:

```
examples/
├── deploy-mainnet-follower.md
├── troubleshoot-sync-issues.md
└── monitor-node-fleet.md
```

---

## Quality Checklist

Before considering prompts complete:

### Design Quality
- [ ] Each prompt combines 2+ tools into a coherent workflow
- [ ] Prompts encode best practices from documentation
- [ ] Arguments are minimal but sufficient
- [ ] Optional arguments have sensible defaults
- [ ] Prompts cover main user workflows (80/20 rule)

### Implementation Quality
- [ ] All prompts have unit tests
- [ ] Integration tests pass with real data
- [ ] Required arguments are validated
- [ ] Error messages are helpful
- [ ] Templates render correctly with all arg combinations

### Documentation Quality
- [ ] Each prompt has clear description
- [ ] Arguments are documented
- [ ] Examples are provided
- [ ] "When to use" guidance is clear
- [ ] Related prompts are cross-referenced

### User Value
- [ ] Prompts save significant time vs. manual tool calls
- [ ] Instructions are actionable (not just informational)
- [ ] Common errors are anticipated and addressed
- [ ] Prompts guide users to success

---

## Example: Applying This Process

### Case Study: Accumulate Follower Deployment

**Step 1: Analysis**
- Repository: accumulate (core protocol)
- MCP tools: 40+ tools for node operations
- Users: Node operators deploying followers
- Common task: "Deploy a follower node"

**Step 2: Workflow Identification**
- Workflow: Deploy follower
- Steps: Prerequisites → Download → Configure → Init → Start → Verify
- Tools needed: 8-10 different tools

**Step 3: Design**
- Prompt: `deploy-follower-node`
- Args: `network`, `node_name`, `data_dir` (optional)
- Pattern: Step-by-step workflow with validation

**Step 4: Implementation**
- Created `prompts.go` in accumulate MCP server
- Added to server's prompt handlers
- Registered in server initialization

**Step 5: Testing**
- Unit tests: Template rendering, arg validation
- Integration: Tested with testnet deployment
- Manual: Deployed actual follower using prompt

**Step 6: Documentation**
- Added to prompts-summary.md
- Created example in examples/deploy-mainnet-follower.md
- Updated README with prompt reference

**Result**:
- Reduced deployment from 15+ manual steps to 1 prompt call
- Encoded best practices (peer selection, validation)
- New operators can deploy successfully on first try

---

## Templates

### Prompt Design Template

```go
{
    Name:        "ACTION-TARGET",  // e.g., deploy-follower, check-health
    Description: "One-line purpose",
    Arguments: []PromptArgument{
        {
            Name:        "required_arg",
            Description: "What this is for",
            Required:    true,
        },
        {
            Name:        "optional_arg",
            Description: "Optional parameter",
            Required:    false,
        },
    },
    Template: func(args map[string]string) string {
        // Handle optional args with defaults
        optVal := args["optional_arg"]
        if optVal == "" {
            optVal = "default-value"
        }

        return `[Task name]: ` + args["required_arg"] + `

**Prerequisites:**
- Requirement 1
- Requirement 2

**Steps:**
1. Use \`tool-name\` with args...
2. Verify result...

**Validation:**
- [ ] Check 1
- [ ] Check 2

**Troubleshooting:**
If X fails:
  - Try Y

**Next Steps:**
- Related prompt: "next-action"
`
    },
}
```

### Test Template

```go
func TestPromptName(t *testing.T) {
    prompts := GetPrompts()

    var prompt *PromptDefinition
    for i := range prompts {
        if prompts[i].Name == "prompt-name" {
            prompt = &prompts[i]
            break
        }
    }

    if prompt == nil {
        t.Fatal("Prompt not found")
    }

    // Test with valid args
    args := map[string]string{
        "required_arg": "test-value",
    }

    result := prompt.Template(args)

    // Verify substitution
    if !strings.Contains(result, "test-value") {
        t.Error("Template didn't substitute argument")
    }

    // Verify key content
    if !strings.Contains(result, "Prerequisites") {
        t.Error("Template missing prerequisites section")
    }
}
```

---

## Tips & Best Practices

### Do's ✅

- Start with user workflows, not tools
- Encode organizational knowledge (CLAUDE.md, best practices)
- Test with real data and scenarios
- Provide troubleshooting guidance
- Link related prompts
- Use clear, imperative language
- Include validation steps
- Specify expected outcomes

### Don'ts ❌

- Don't create prompts that just wrap one tool
- Don't make users memorize argument formats
- Don't assume users know tool sequences
- Don't skip error handling
- Don't forget optional argument defaults
- Don't write walls of text (use structure)
- Don't skip testing with real data
- Don't leave prompts undocumented

### Prompt Naming Conventions

```
[ACTION]-[TARGET]

Examples:
- deploy-follower-node      ✅ Clear action + target
- check-sync-status         ✅ Clear action + target
- troubleshoot-network      ✅ Clear action + scope
- node-deploy               ❌ Target-action (backwards)
- deployment                ❌ No action (ambiguous)
- do-the-thing             ❌ Vague
```

---

## Success Metrics

How to know if your prompts are successful:

### Quantitative
- **Usage**: Prompts are called more than individual tools
- **Coverage**: Top 5 workflows have prompts
- **Efficiency**: Task completion time reduced 50%+

### Qualitative
- **Discoverability**: Users find prompts via list
- **Clarity**: Users understand what prompt does
- **Completeness**: Prompt guides to successful completion
- **Reusability**: Same prompt works for multiple users/scenarios

### Feedback Collection
- Monitor which prompts are used most
- Track which fail/error most often
- Ask users: "Did this prompt help?"
- Review issues referencing prompts
- Update prompts based on feedback

---

## Appendix: Repository-Specific Examples

### For Node Deployment Repositories
- `deploy-follower-node`
- `upgrade-node-version`
- `backup-node-data`
- `restore-from-snapshot`
- `monitor-sync-status`
- `diagnose-peer-issues`
- `configure-monitoring`

### For Data/API Repositories
- `query-historical-data`
- `generate-report-for-period`
- `export-data-to-format`
- `validate-data-integrity`
- `benchmark-query-performance`

### For Build/Deploy Repositories
- `setup-dev-environment`
- `run-integration-tests`
- `deploy-to-environment`
- `rollback-deployment`
- `check-deployment-health`

---

## Next Steps

After completing this process:

1. **Iterate**: Collect feedback and improve prompts
2. **Expand**: Add prompts for edge cases
3. **Share**: Document patterns for other repos
4. **Maintain**: Update prompts when tools change
5. **Monitor**: Track usage and effectiveness

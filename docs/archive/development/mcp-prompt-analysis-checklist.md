# Prompt Analysis Checklist

Quick reference checklist for analyzing a repository and creating MCP prompts.

## Phase 1: Discovery (30-60 min)

### Repository Understanding
- [ ] Read README.md - What does this repo do?
- [ ] Read CLAUDE.md - What are the workflows?
- [ ] Read MCP design docs - What tools exist?
- [ ] Read active.md - What are people doing now?
- [ ] Read work-history.md - What patterns exist?

### MCP Inventory
- [ ] List all tools (run `ListTools()` or read MCP_DESIGN.md)
- [ ] List all resources (URIs available)
- [ ] Note what data can be accessed

**Output**: Inventory table of tools and resources

---

## Phase 2: Workflow Analysis (60-90 min)

### Identify Common Tasks
- [ ] Review GitHub/GitLab issues - What do people ask?
- [ ] Check documentation examples - What's explained often?
- [ ] Talk to users (if possible) - What's painful?
- [ ] List top 10 common tasks

### Map Workflows
For each common task:
- [ ] Break into steps
- [ ] Identify which tools are needed
- [ ] Note prerequisites
- [ ] Note validation steps
- [ ] Note common problems

**Output**: Workflow diagrams mapping tasks → steps → tools

---

## Phase 3: Prompt Design (2-3 hours)

### Select Prompts to Create
Prioritize prompts that:
- [ ] Combine 2+ tools
- [ ] Cover frequent workflows (80/20 rule)
- [ ] Encode best practices
- [ ] Reduce complexity significantly

Target: 5-15 prompts per repository

### Design Each Prompt
For each prompt:
- [ ] Name: `[action]-[target]` format
- [ ] Description: One-line purpose
- [ ] Arguments: List required and optional
- [ ] Template structure:
  - [ ] Introduction/context
  - [ ] Prerequisites
  - [ ] Numbered steps with tool calls
  - [ ] Validation checklist
  - [ ] Troubleshooting guide
  - [ ] Next steps/related prompts

**Output**: Prompt specifications (can be in markdown before coding)

---

## Phase 4: Implementation (3-4 hours)

### Code Prompts
- [ ] Create `prompts.go`
- [ ] Define `PromptDefinition` struct
- [ ] Define `PromptArgument` struct
- [ ] Implement `GetPrompts()` function
- [ ] Write template functions

### Integrate with Server
- [ ] Add `ListPrompts()` method
- [ ] Add `GetPrompt(name, args)` method
- [ ] Add `registerPrompts()` to server init
- [ ] Update server initialization

**Output**: Working prompt code integrated with MCP server

---

## Phase 5: Testing (2-3 hours)

### Unit Tests
- [ ] Test `GetPrompts()` returns all prompts
- [ ] Test prompt names are unique
- [ ] Test all prompts have descriptions
- [ ] Test all prompts have templates
- [ ] Test required argument validation
- [ ] Test template rendering with args
- [ ] Test missing argument errors

### Integration Tests
- [ ] Test with real repository paths
- [ ] Test each workflow prompt end-to-end
- [ ] Test with realistic arguments
- [ ] Test optional argument defaults
- [ ] Verify output length is reasonable (>100 chars)

### Manual Testing
- [ ] Deploy to Claude Desktop
- [ ] List prompts (verify all appear)
- [ ] Execute each prompt category
- [ ] Verify instructions are clear
- [ ] Follow a prompt to completion
- [ ] Test with edge case arguments

**Output**: Passing test suite + manual validation notes

---

## Phase 6: Documentation (1-2 hours)

### Create Prompt Catalog
- [ ] Create `prompts-summary.md`
- [ ] Document each prompt:
  - [ ] Name and purpose
  - [ ] Arguments (required/optional)
  - [ ] When to use
  - [ ] Example arguments
  - [ ] What it does (steps)
- [ ] Group prompts by category
- [ ] Add usage patterns section

### Update Main Docs
- [ ] Add prompts section to README.md
- [ ] Link to prompt catalog
- [ ] Add quick start example
- [ ] Update CLAUDE.md if prompts encode workflows

### Create Examples
- [ ] Create `examples/` directory
- [ ] Add 2-3 real-world use case examples
- [ ] Show prompt usage in context

**Output**: Complete documentation

---

## Quality Gates

Before marking complete:

### Design Quality
- [ ] 5-15 prompts created (not too few, not too many)
- [ ] Each prompt combines 2+ tools
- [ ] Prompts cover top workflows
- [ ] Arguments are minimal but sufficient
- [ ] Templates follow consistent structure

### Implementation Quality
- [ ] All unit tests pass
- [ ] Integration tests pass
- [ ] `go vet` passes
- [ ] No lint warnings
- [ ] Code follows repository patterns

### Documentation Quality
- [ ] All prompts documented
- [ ] Examples provided
- [ ] Clear usage guidance
- [ ] Proper markdown formatting

### User Value
- [ ] Prompts save time vs. manual approach
- [ ] Instructions are actionable
- [ ] Common errors addressed
- [ ] Success path is clear

---

## Time Estimates

| Phase | Time | Can Parallelize? |
|-------|------|------------------|
| Discovery | 30-60 min | No |
| Workflow Analysis | 60-90 min | Partially |
| Prompt Design | 2-3 hours | Yes (multiple prompts) |
| Implementation | 3-4 hours | Yes (code + integration) |
| Testing | 2-3 hours | Partially |
| Documentation | 1-2 hours | Yes (catalog + examples) |
| **Total** | **8-13 hours** | |

Parallelization opportunity: Multiple people can design/implement different prompts simultaneously after Phase 2.

---

## Quick Start Template

Use this when starting a new repository analysis:

```markdown
# Prompt Analysis: [Repository Name]

## Repository Info
- **Purpose**: [What this repo does]
- **Users**: [Who uses it]
- **MCP Status**: [Has MCP? How many tools?]

## Tools Inventory
| Tool | Purpose |
|------|---------|
| tool-1 | ... |
| tool-2 | ... |

## Common Tasks
1. [Task 1]
2. [Task 2]
3. [Task 3]

## Proposed Prompts
1. **prompt-name-1** - [Purpose]
   - Combines: [tool-1, tool-2, tool-3]
   - Args: [arg1 (req), arg2 (opt)]

2. **prompt-name-2** - [Purpose]
   - Combines: [tool-4, tool-5]
   - Args: [arg1 (req)]

## Priority
- High: [prompts for most common tasks]
- Medium: [important but less frequent]
- Low: [nice to have]
```

---

## Examples by Repository Type

### Node Deployment Repository (e.g., accumulate)
**Focus Areas**: Deployment, monitoring, troubleshooting
```
Suggested Prompts:
- deploy-follower-node
- upgrade-node
- check-node-health
- diagnose-sync-issues
- backup-and-restore
- configure-monitoring
```

### Data/Calculation Repository (e.g., staking)
**Focus Areas**: Queries, calculations, reports
```
Suggested Prompts:
- calculate-rewards-for-period
- query-account-history
- generate-staking-report
- validate-calculations
- export-data
```

### API/Service Repository (e.g., launch-api-server)
**Focus Areas**: Deployment, configuration, testing
```
Suggested Prompts:
- deploy-api-server
- configure-endpoints
- test-api-health
- troubleshoot-requests
- scale-deployment
```

### Build/CI Repository
**Focus Areas**: Setup, testing, deployment
```
Suggested Prompts:
- setup-dev-environment
- run-test-suite
- build-and-deploy
- validate-build
- troubleshoot-ci-failures
```

---

## Common Pitfalls to Avoid

❌ **Creating too many prompts**
- Start with 5-10, can always add more
- Focus on 80/20: Most common tasks

❌ **Prompts that just wrap one tool**
- Only create prompts that combine multiple tools
- Single-tool calls should stay as direct tool usage

❌ **Missing error handling**
- Always include troubleshooting section
- Address common failure modes

❌ **Assuming user knowledge**
- Don't assume users know tool names
- Explain what each step does

❌ **Hardcoding values**
- Use arguments for variability
- Provide sensible defaults

❌ **Forgetting validation**
- Always include verification steps
- Provide success criteria

❌ **No real-world testing**
- Don't just unit test
- Actually use prompts for real tasks

---

## Success Indicators

You'll know prompts are successful when:

✅ Users prefer prompts over direct tool calls
✅ Onboarding time decreases (new users succeed faster)
✅ Support questions decrease (prompts answer them)
✅ Prompts are referenced in documentation
✅ Workflows become standardized
✅ Fewer errors in common tasks

---

## Iteration and Improvement

After initial release:

### Week 1-2
- [ ] Monitor which prompts are used most
- [ ] Collect feedback on clarity
- [ ] Fix any errors or confusing instructions

### Month 1
- [ ] Analyze usage patterns
- [ ] Identify missing prompts
- [ ] Refine based on feedback

### Ongoing
- [ ] Update prompts when tools change
- [ ] Add prompts for new workflows
- [ ] Remove unused prompts
- [ ] Share successful patterns across repos

---

## Reference Links

- Full process doc: `prompt-analysis-process.md`
- Tracking MCP example: `prompts.go`
- Test examples: `prompts_test.go`
- Documentation template: `prompts-summary.md`

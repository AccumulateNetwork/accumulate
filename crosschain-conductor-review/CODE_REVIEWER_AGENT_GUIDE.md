# Code Reviewer Agent Implementation Guide

## Overview
This guide explains how to create a custom code reviewer agent that can automatically review code changes, identify issues, and suggest improvements.

## Quick Start

### Option 1: Using Claude Code SDK (Python)

```python
from claude_code import ClaudeSDKClient, ClaudeCodeOptions
import asyncio

async def create_code_reviewer():
    """Create a specialized code reviewer agent"""
    
    options = ClaudeCodeOptions(
        system_prompt="""You are an expert code reviewer focusing on:
        1. Security vulnerabilities
        2. Performance issues
        3. Code quality and maintainability
        4. Best practices and design patterns
        5. Error handling and edge cases
        
        Provide actionable feedback with specific line references.
        Prioritize critical issues over style preferences.""",
        
        max_turns=5,  # Allow multiple review iterations
        
        allowed_tools=[
            "Read",      # Read source files
            "Grep",      # Search for patterns
            "Bash",      # Run linters/tests
            "WebSearch", # Look up best practices
            "TodoWrite"  # Track issues found
        ]
    )
    
    async with ClaudeSDKClient(options=options) as client:
        # Review a specific PR or codebase
        result = await client.query(
            "Review the recent changes in the crosschain conductor implementation"
        )
        return result

# Run the reviewer
asyncio.run(create_code_reviewer())
```

### Option 2: Using Task Tool with Subagents

```python
from claude_code import Task

def launch_code_review(file_paths, review_type="comprehensive"):
    """Launch a code review using the Task tool"""
    
    prompt = f"""
    Perform a {review_type} code review of the following files:
    {', '.join(file_paths)}
    
    Focus on:
    1. Critical bugs and security issues
    2. Performance bottlenecks
    3. Code quality and maintainability
    4. Test coverage gaps
    5. Documentation needs
    
    For each issue found:
    - Specify the file and line number
    - Explain the problem
    - Suggest a fix
    - Rate severity (Critical/High/Medium/Low)
    
    Create a summary report with:
    - Total issues by severity
    - Must-fix items before merge
    - Recommended improvements
    """
    
    return Task(
        description="Code Review",
        prompt=prompt,
        subagent_type="general-purpose"
    )
```

## Advanced Code Reviewer Configuration

### Security-Focused Reviewer

```python
class SecurityReviewer:
    def __init__(self):
        self.security_patterns = {
            "sql_injection": r"(SELECT|INSERT|UPDATE|DELETE).*\+.*input",
            "xss": r"innerHTML|document\.write|eval\(",
            "path_traversal": r"\.\./|\.\.\\",
            "hardcoded_secrets": r"(api_key|password|secret|token)\s*=\s*[\"'][^\"']+[\"']",
            "command_injection": r"exec\(|system\(|eval\(|subprocess\.call",
        }
    
    async def review_security(self, file_path):
        """Perform security-focused code review"""
        
        options = ClaudeCodeOptions(
            system_prompt="""You are a security expert reviewing code for vulnerabilities.
            Check for: OWASP Top 10, injection flaws, authentication issues, 
            sensitive data exposure, XXE, broken access control, security misconfig,
            XSS, insecure deserialization, using components with known vulnerabilities,
            insufficient logging.""",
            
            allowed_tools=["Read", "Grep", "WebSearch"]
        )
        
        async with ClaudeSDKClient(options=options) as client:
            # First scan for known patterns
            for pattern_name, regex in self.security_patterns.items():
                result = await client.grep(regex, file_path)
                if result:
                    await client.query(f"Analyze {pattern_name} risk in: {result}")
            
            # Deep security analysis
            return await client.query(f"Perform deep security review of {file_path}")
```

### Performance Reviewer

```python
class PerformanceReviewer:
    async def review_performance(self, file_path):
        """Review code for performance issues"""
        
        options = ClaudeCodeOptions(
            system_prompt="""You are a performance expert. Identify:
            1. O(n²) or worse algorithms
            2. Unnecessary database queries (N+1 problems)
            3. Missing indexes or caching
            4. Memory leaks or unbounded growth
            5. Blocking I/O in async code
            6. Inefficient data structures
            7. Missing pagination or limits""",
            
            allowed_tools=["Read", "Grep", "Bash"]
        )
        
        async with ClaudeSDKClient(options=options) as client:
            # Analyze complexity
            await client.query(f"Analyze algorithmic complexity in {file_path}")
            
            # Check for common performance anti-patterns
            patterns = [
                "for.*for",  # Nested loops
                "await.*await",  # Sequential awaits
                "SELECT.*FROM.*JOIN.*JOIN",  # Complex queries
            ]
            
            for pattern in patterns:
                await client.grep(pattern, file_path)
            
            return await client.query("Summarize performance findings")
```

## Integration with Git Hooks

### Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Run code reviewer on staged files
staged_files=$(git diff --cached --name-only --diff-filter=ACM | grep -E '\.(py|js|go)$')

if [ -n "$staged_files" ]; then
    echo "Running code review on staged files..."
    
    claude-code review \
        --type "pre-commit" \
        --files "$staged_files" \
        --severity "high" \
        --fix-in-place
    
    if [ $? -ne 0 ]; then
        echo "Code review found critical issues. Please fix before committing."
        exit 1
    fi
fi
```

### GitHub Actions Integration

```yaml
name: Automated Code Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  code-review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Claude Code
        run: |
          pip install claude-code-sdk
          export ANTHROPIC_API_KEY=${{ secrets.ANTHROPIC_API_KEY }}
      
      - name: Run Code Review
        run: |
          python -c "
          import asyncio
          from claude_code import ClaudeSDKClient, ClaudeCodeOptions
          
          async def review():
              options = ClaudeCodeOptions(
                  system_prompt='Review PR for issues',
                  allowed_tools=['Read', 'Grep']
              )
              async with ClaudeSDKClient(options=options) as client:
                  result = await client.query('Review changed files')
                  print(result)
          
          asyncio.run(review())
          "
      
      - name: Post Review Comments
        if: always()
        run: |
          # Post review results as PR comments
          gh pr comment --body-file review_results.md
```

## Custom Review Rules

### Define Project-Specific Rules

```python
class CustomReviewer:
    def __init__(self, rules_file="review_rules.yaml"):
        self.rules = self.load_rules(rules_file)
    
    def load_rules(self, file_path):
        """Load custom review rules from YAML"""
        import yaml
        with open(file_path, 'r') as f:
            return yaml.safe_load(f)
    
    async def apply_custom_rules(self, file_path):
        """Apply project-specific review rules"""
        
        options = ClaudeCodeOptions(
            system_prompt=f"""Apply these custom review rules:
            {self.rules}
            
            Check for compliance and suggest fixes.""",
            allowed_tools=["Read", "Grep", "Edit"]
        )
        
        async with ClaudeSDKClient(options=options) as client:
            for rule in self.rules['rules']:
                if rule['type'] == 'pattern':
                    await client.grep(rule['regex'], file_path)
                elif rule['type'] == 'structure':
                    await client.query(f"Check {rule['description']} in {file_path}")
            
            return await client.query("Generate review report")
```

### Example Rules File (review_rules.yaml)

```yaml
rules:
  - name: "No console.log in production"
    type: pattern
    regex: "console\\.(log|debug|trace)"
    severity: high
    fix: "Replace with proper logging framework"
  
  - name: "Require error handling"
    type: structure
    description: "All async functions must have try-catch"
    severity: critical
  
  - name: "Document public APIs"
    type: structure
    description: "All public functions must have JSDoc/docstrings"
    severity: medium
  
  - name: "Limit function complexity"
    type: metric
    max_complexity: 10
    severity: high
```

## Testing the Code Reviewer

```python
import unittest
from unittest.mock import Mock, patch

class TestCodeReviewer(unittest.TestCase):
    def test_security_patterns_detected(self):
        """Test that security patterns are properly detected"""
        reviewer = SecurityReviewer()
        
        vulnerable_code = '''
        query = "SELECT * FROM users WHERE id = " + user_input
        document.innerHTML = untrusted_data
        api_key = "sk-1234567890abcdef"
        '''
        
        # Mock file with vulnerable code
        with patch('claude_code.Read', return_value=vulnerable_code):
            result = asyncio.run(reviewer.review_security('test.py'))
            
            self.assertIn('sql_injection', result)
            self.assertIn('xss', result)
            self.assertIn('hardcoded_secrets', result)
```

## Best Practices

1. **Start Specific**: Focus on one type of review (security, performance, style)
2. **Iterate**: Use max_turns to allow follow-up questions
3. **Provide Context**: Include project conventions in system prompt
4. **Rate Limit**: Don't review huge codebases at once
5. **Cache Results**: Store reviews to avoid re-analyzing unchanged code
6. **Actionable Feedback**: Always suggest specific fixes
7. **Prioritize**: Focus on critical issues first

## Troubleshooting

### Common Issues

1. **Agent times out on large files**
   - Solution: Break into smaller chunks or increase timeout
   
2. **Too many false positives**
   - Solution: Refine system prompt with specific examples
   
3. **Missing context about project**
   - Solution: Include project README or conventions in prompt

4. **Inconsistent reviews**
   - Solution: Use structured output format and validation

## Next Steps

1. Customize the system prompt for your project needs
2. Add specific patterns for your tech stack
3. Integrate with your CI/CD pipeline
4. Create review templates for different scenarios
5. Build a feedback loop to improve accuracy

Remember: The code reviewer agent is a tool to assist, not replace, human reviewers. Always validate critical findings manually.
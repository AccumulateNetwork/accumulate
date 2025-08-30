#!/usr/bin/env python3
"""
Design-Driven Code Reviewer Agent
Maintains design documents and ensures code compliance
"""

import os
import sys
import json
import argparse
import subprocess
import re
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Tuple
import difflib

class DesignReviewer:
    def __init__(self, config_path: Optional[str] = None):
        self.config = self.load_config(config_path)
        self.design_dir = Path(self.config.get('design_dir', 'docs/designs'))
        self.template_dir = Path(self.config.get('template_dir', 'tools/design-reviewer/templates'))
        
    def load_config(self, config_path: Optional[str]) -> Dict:
        """Load configuration from file or use defaults"""
        default_config = {
            'design_dir': 'docs/designs',
            'template_dir': 'tools/design-reviewer/templates',
            'strict_mode': True,
            'require_tests': True
        }
        
        if config_path and Path(config_path).exists():
            with open(config_path, 'r') as f:
                return {**default_config, **json.load(f)}
        return default_config
    
    def create_design(self, issue_id: str, issue_title: str = "", issue_body: str = "") -> str:
        """Create a design document for an issue"""
        design_path = self.design_dir / f"issue-{issue_id}-design.md"
        
        # Create design directory if it doesn't exist
        self.design_dir.mkdir(parents=True, exist_ok=True)
        
        # Generate design document
        design_content = self._generate_design_template(issue_id, issue_title, issue_body)
        
        with open(design_path, 'w') as f:
            f.write(design_content)
        
        return str(design_path)
    
    def _generate_design_template(self, issue_id: str, title: str, body: str) -> str:
        """Generate design document from template"""
        template = f"""# Issue Design Document: #{issue_id}

## Issue Summary
- **Issue ID**: #{issue_id}
- **Title**: {title or '[Add title]'}
- **Status**: Draft
- **Created**: {datetime.now().strftime('%Y-%m-%d')}
- **Last Updated**: {datetime.now().strftime('%Y-%m-%d')}

## Problem Statement
{body or '[Describe the problem being solved]'}

## Design Specification

### Architecture Overview
[Describe high-level architecture and component interaction]

### Component Definitions

#### Affected Files
```
[List files that will be modified]
```

#### New Components
```
[List new files/modules to be created]
```

### API Contracts

#### Functions/Methods
```go
// Example function signature
func ProcessTransaction(tx *Transaction) error {{
    // Implementation details
}}
```

#### Data Structures
```go
// Example data structure
type Component struct {{
    Field1 string
    Field2 int
}}
```

### Data Flow
1. [Step 1: Data enters system]
2. [Step 2: Processing occurs]
3. [Step 3: Results produced]

### Error Handling
- **Error Condition 1**: [Description and handling]
- **Error Condition 2**: [Description and handling]

### Testing Requirements
- [ ] Unit tests for new functions
- [ ] Integration tests for workflows
- [ ] Edge case coverage
- [ ] Error condition tests

## Implementation Checklist
- [ ] Component structure matches design
- [ ] API signatures match specification
- [ ] Data flow implemented as designed
- [ ] Error handling follows specification
- [ ] Tests cover required scenarios
- [ ] Documentation updated

## Acceptance Criteria
1. [Criterion 1]
2. [Criterion 2]
3. [Criterion 3]

## Change Log
- {datetime.now().strftime('%Y-%m-%d')}: Initial design created
"""
        return template
    
    def review_code(self, design_path: str, branch: str = None, pr_id: str = None) -> Dict:
        """Review code against design document"""
        if not Path(design_path).exists():
            raise FileNotFoundError(f"Design document not found: {design_path}")
        
        # Load design document
        with open(design_path, 'r') as f:
            design_content = f.read()
        
        # Parse design requirements
        requirements = self._parse_design_requirements(design_content)
        
        # Get code changes
        changes = self._get_code_changes(branch, pr_id)
        
        # Perform compliance check
        compliance_report = self._check_compliance(requirements, changes)
        
        return compliance_report
    
    def _parse_design_requirements(self, design_content: str) -> Dict:
        """Extract requirements from design document"""
        requirements = {
            'api_contracts': [],
            'data_structures': [],
            'affected_files': [],
            'test_requirements': [],
            'acceptance_criteria': []
        }
        
        # Extract API contracts (function signatures)
        api_pattern = r'func\s+(\w+)\([^)]*\)[^{]*'
        requirements['api_contracts'] = re.findall(api_pattern, design_content)
        
        # Extract data structures
        struct_pattern = r'type\s+(\w+)\s+struct'
        requirements['data_structures'] = re.findall(struct_pattern, design_content)
        
        # Extract test requirements (checklist items)
        test_pattern = r'- \[.\] (.+?)(?:\n|$)'
        in_testing_section = False
        for line in design_content.split('\n'):
            if 'Testing Requirements' in line:
                in_testing_section = True
            elif '##' in line and in_testing_section:
                in_testing_section = False
            elif in_testing_section:
                match = re.match(test_pattern, line)
                if match:
                    requirements['test_requirements'].append(match.group(1))
        
        return requirements
    
    def _get_code_changes(self, branch: Optional[str], pr_id: Optional[str]) -> Dict:
        """Get code changes from branch or PR"""
        changes = {
            'modified_files': [],
            'added_functions': [],
            'modified_functions': [],
            'added_structs': [],
            'test_files': []
        }
        
        # Get diff
        if branch:
            diff_cmd = f"git diff main...{branch}"
        else:
            diff_cmd = "git diff HEAD~1"
        
        try:
            result = subprocess.run(diff_cmd, shell=True, capture_output=True, text=True)
            diff_content = result.stdout
            
            # Parse diff for changes
            current_file = None
            for line in diff_content.split('\n'):
                if line.startswith('+++'):
                    current_file = line[6:]
                    if current_file and current_file != '/dev/null':
                        changes['modified_files'].append(current_file)
                        if '_test.go' in current_file:
                            changes['test_files'].append(current_file)
                elif line.startswith('+func '):
                    func_match = re.match(r'\+func\s+(\w+)', line)
                    if func_match:
                        changes['added_functions'].append(func_match.group(1))
                elif line.startswith('+type ') and 'struct' in line:
                    struct_match = re.match(r'\+type\s+(\w+)\s+struct', line)
                    if struct_match:
                        changes['added_structs'].append(struct_match.group(1))
        except subprocess.CalledProcessError:
            pass
        
        return changes
    
    def _check_compliance(self, requirements: Dict, changes: Dict) -> Dict:
        """Check if changes comply with design requirements"""
        report = {
            'compliance_score': 0,
            'total_checks': 0,
            'passed_checks': 0,
            'critical_issues': [],
            'warnings': [],
            'compliant_areas': [],
            'missing_items': []
        }
        
        # Check API contracts
        for func in requirements['api_contracts']:
            report['total_checks'] += 1
            if func in changes['added_functions'] or func in changes['modified_functions']:
                report['passed_checks'] += 1
                report['compliant_areas'].append(f"Function '{func}' implemented")
            else:
                report['missing_items'].append(f"Function '{func}' not found")
        
        # Check data structures
        for struct in requirements['data_structures']:
            report['total_checks'] += 1
            if struct in changes['added_structs']:
                report['passed_checks'] += 1
                report['compliant_areas'].append(f"Struct '{struct}' implemented")
            else:
                report['missing_items'].append(f"Struct '{struct}' not found")
        
        # Check test coverage
        if requirements['test_requirements'] and not changes['test_files']:
            report['critical_issues'].append("No test files found")
        elif changes['test_files']:
            report['compliant_areas'].append(f"Test files added: {len(changes['test_files'])}")
        
        # Calculate compliance score
        if report['total_checks'] > 0:
            report['compliance_score'] = (report['passed_checks'] / report['total_checks']) * 100
        
        return report
    
    def generate_report(self, design_path: str, compliance_report: Dict) -> str:
        """Generate a formatted review report"""
        report_path = design_path.replace('-design.md', '-review.md')
        
        report_content = f"""# Code Review Report

## Design Document: {Path(design_path).name}
**Generated**: {datetime.now().strftime('%Y-%m-%d %H:%M')}

## Design Compliance Summary
- **Overall Compliance**: {compliance_report['compliance_score']:.1f}%
- **Checks Passed**: {compliance_report['passed_checks']}/{compliance_report['total_checks']}
- **Critical Issues**: {len(compliance_report['critical_issues'])}
- **Warnings**: {len(compliance_report['warnings'])}

## Detailed Analysis

### ✅ Compliant Areas
{self._format_list(compliance_report['compliant_areas'])}

### ❌ Missing Implementation
{self._format_list(compliance_report['missing_items'])}

### 🚨 Critical Issues
{self._format_list(compliance_report['critical_issues'])}

### ⚠️ Warnings
{self._format_list(compliance_report['warnings'])}

## Recommendations
"""
        
        if compliance_report['compliance_score'] >= 90:
            report_content += "- ✅ Code meets design requirements\n"
            report_content += "- Consider minor improvements listed above\n"
        elif compliance_report['compliance_score'] >= 70:
            report_content += "- ⚠️ Code partially meets design requirements\n"
            report_content += "- Address missing implementations before approval\n"
        else:
            report_content += "- ❌ Code does not meet design requirements\n"
            report_content += "- Significant work needed to match design specification\n"
        
        # Add specific recommendations based on issues
        if compliance_report['critical_issues']:
            report_content += "\n### Required Actions:\n"
            for issue in compliance_report['critical_issues']:
                report_content += f"1. Fix: {issue}\n"
        
        with open(report_path, 'w') as f:
            f.write(report_content)
        
        return report_path
    
    def _format_list(self, items: List[str]) -> str:
        """Format a list for markdown output"""
        if not items:
            return "- None\n"
        return '\n'.join(f"- {item}" for item in items) + '\n'
    
    def update_design(self, design_path: str, changes: str) -> None:
        """Update design document with approved changes"""
        with open(design_path, 'r') as f:
            content = f.read()
        
        # Add to change log
        change_log_marker = "## Change Log"
        if change_log_marker in content:
            date_str = datetime.now().strftime('%Y-%m-%d')
            change_entry = f"- {date_str}: {changes}\n"
            content = content.replace(change_log_marker, 
                                     f"{change_log_marker}\n{change_entry}")
        
        # Update last updated date
        content = re.sub(
            r'- \*\*Last Updated\*\*: .+',
            f"- **Last Updated**: {datetime.now().strftime('%Y-%m-%d')}",
            content
        )
        
        with open(design_path, 'w') as f:
            f.write(content)

def main():
    parser = argparse.ArgumentParser(description='Design-Driven Code Reviewer')
    subparsers = parser.add_subparsers(dest='command', help='Commands')
    
    # Create design command
    create_parser = subparsers.add_parser('create', help='Create design document')
    create_parser.add_argument('--issue', required=True, help='Issue ID')
    create_parser.add_argument('--title', help='Issue title')
    create_parser.add_argument('--body', help='Issue description')
    
    # Review code command
    review_parser = subparsers.add_parser('review', help='Review code against design')
    review_parser.add_argument('--design', required=True, help='Path to design document')
    review_parser.add_argument('--branch', help='Branch to review')
    review_parser.add_argument('--pr', help='PR ID to review')
    
    # Update design command
    update_parser = subparsers.add_parser('update', help='Update design document')
    update_parser.add_argument('--design', required=True, help='Path to design document')
    update_parser.add_argument('--changes', required=True, help='Description of changes')
    
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        return
    
    reviewer = DesignReviewer()
    
    if args.command == 'create':
        design_path = reviewer.create_design(args.issue, args.title or "", args.body or "")
        print(f"✅ Design document created: {design_path}")
        print(f"📝 Next step: Edit the design document to add specifications")
        
    elif args.command == 'review':
        try:
            compliance = reviewer.review_code(args.design, args.branch, args.pr)
            report_path = reviewer.generate_report(args.design, compliance)
            print(f"📊 Compliance Score: {compliance['compliance_score']:.1f}%")
            print(f"✅ Passed: {compliance['passed_checks']}/{compliance['total_checks']} checks")
            if compliance['critical_issues']:
                print(f"🚨 Critical Issues: {len(compliance['critical_issues'])}")
            print(f"📄 Report saved: {report_path}")
        except Exception as e:
            print(f"❌ Error: {e}")
            sys.exit(1)
            
    elif args.command == 'update':
        reviewer.update_design(args.design, args.changes)
        print(f"✅ Design document updated")

if __name__ == '__main__':
    main()
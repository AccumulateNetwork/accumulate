# Design-Driven Code Reviewer Agent

## Purpose
Maintain design documents for issues and ensure code implementations match the approved design specifications.

## Agent Structure

### 1. Design Document Management

#### Design Document Template
```markdown
# Issue Design Document: [Issue #XXX]

## Issue Summary
- **Issue ID**: #XXX
- **Title**: [Issue title]
- **Status**: [Draft/Approved/In Progress/Implemented]
- **Created**: [Date]
- **Last Updated**: [Date]

## Problem Statement
[Clear description of the problem being solved]

## Design Specification

### Architecture Overview
[High-level architecture and component interaction]

### Component Definitions
[Detailed component specifications]

### API Contracts
[Interface definitions, function signatures, data structures]

### Data Flow
[How data moves through the system]

### Error Handling
[Expected error conditions and handling strategies]

### Testing Requirements
[Required test coverage and scenarios]

## Implementation Checklist
- [ ] Component structure matches design
- [ ] API signatures match specification
- [ ] Data flow implemented as designed
- [ ] Error handling follows specification
- [ ] Tests cover required scenarios
- [ ] Documentation updated

## Change Log
[Track design changes and rationale]
```

### 2. Review Process

#### Pre-Implementation Review
1. **Design Document Creation**
   - Create design document from issue requirements
   - Define clear acceptance criteria
   - Specify technical approach
   - Identify affected components

2. **Design Validation**
   - Verify design completeness
   - Check for architectural consistency
   - Validate against existing patterns
   - Identify potential conflicts

#### Implementation Review
1. **Code-to-Design Compliance Check**
   ```yaml
   Review Checklist:
     Structure:
       - [ ] File organization matches design
       - [ ] Component boundaries respected
       - [ ] Dependencies correctly managed
     
     Implementation:
       - [ ] Function signatures match design
       - [ ] Data structures as specified
       - [ ] Algorithms follow design approach
       - [ ] Error handling as designed
     
     Integration:
       - [ ] Interfaces properly implemented
       - [ ] Communication patterns correct
       - [ ] State management as designed
     
     Quality:
       - [ ] Tests match requirements
       - [ ] Documentation complete
       - [ ] Performance considerations met
   ```

2. **Deviation Analysis**
   - Document any deviations from design
   - Justify necessary changes
   - Update design document if approved
   - Track design evolution

### 3. Agent Workflow

#### Phase 1: Design Analysis
```python
def analyze_issue(issue_id):
    """Extract requirements and create initial design"""
    steps = [
        "Parse issue description",
        "Identify affected components",
        "Define technical approach",
        "Create design document",
        "Set acceptance criteria"
    ]
    return design_document
```

#### Phase 2: Implementation Review
```python
def review_implementation(pr_id, design_doc):
    """Compare implementation against design"""
    checks = [
        "Structural compliance",
        "API contract verification",
        "Data flow validation",
        "Error handling review",
        "Test coverage analysis"
    ]
    return review_report
```

#### Phase 3: Continuous Validation
```python
def continuous_review(commit_id, design_doc):
    """Ongoing validation during development"""
    validations = [
        "Incremental change review",
        "Design drift detection",
        "Documentation sync check",
        "Test evolution tracking"
    ]
    return validation_status
```

### 4. Review Report Format

```markdown
# Code Review Report: [PR/Commit]

## Design Compliance Summary
- **Overall Compliance**: [Percentage]
- **Critical Issues**: [Count]
- **Minor Deviations**: [Count]

## Detailed Analysis

### ✅ Compliant Areas
- [List of components/features matching design]

### ⚠️ Deviations Requiring Attention
- **Component**: [Name]
  - **Expected**: [Design specification]
  - **Actual**: [Implementation]
  - **Impact**: [Consequences]
  - **Recommendation**: [Action needed]

### 🔍 Additional Findings
- [Improvements beyond design]
- [Performance considerations]
- [Security observations]

## Test Coverage Analysis
- **Required Tests**: [From design]
- **Implemented Tests**: [Actual]
- **Coverage Gap**: [Missing scenarios]

## Documentation Status
- [ ] Design document updated
- [ ] Code comments complete
- [ ] API documentation current
- [ ] User documentation ready

## Approval Checklist
- [ ] All critical design requirements met
- [ ] Deviations documented and justified
- [ ] Tests provide adequate coverage
- [ ] Documentation complete

## Recommendations
[Specific actions for approval or required changes]
```

### 5. Integration Points

#### Version Control Integration
- Link design documents to issues
- Track design evolution in git
- Maintain design-to-code traceability

#### CI/CD Integration
- Automated design compliance checks
- Design drift detection
- Documentation validation

#### Issue Tracking Integration
- Auto-generate design templates
- Link reviews to issues
- Track implementation progress

### 6. Usage Examples

#### Creating a Design Document
```bash
# Generate design document for issue
design-reviewer create --issue 123

# Review existing code against design
design-reviewer review --pr 456 --design docs/issues/123-design.md

# Validate ongoing changes
design-reviewer validate --commit abc123 --design docs/issues/123-design.md
```

#### Review Commands
```yaml
Commands:
  create:    Generate design document from issue
  review:    Full implementation review
  validate:  Incremental change validation
  update:    Update design with approved changes
  report:    Generate compliance report
```

## Benefits

1. **Traceability**: Clear link from requirements to implementation
2. **Quality**: Systematic verification against specifications
3. **Documentation**: Automatic maintenance of design docs
4. **Consistency**: Enforced architectural patterns
5. **Evolution**: Tracked design changes over time

## Configuration

```yaml
design_reviewer:
  templates:
    design: templates/design-document.md
    review: templates/review-report.md
  
  rules:
    strict_mode: true
    allow_deviations: with_justification
    require_tests: true
    
  integration:
    vcs: git
    issues: github/gitlab
    ci: jenkins/github-actions
```
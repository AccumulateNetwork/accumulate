# Document Optimization Plan for AI Use

## Current Issues

### Structure Problems
- **Mixed Scope**: Network config + Command docs + Deployment scripts in one file
- **Inconsistent Hierarchy**: Irregular heading depths (H1-H6)
- **Poor Searchability**: No clear categorization or tagging
- **Redundant Content**: Repeated examples and explanations
- **Length**: 1,095 lines - too long for efficient AI processing

### AI Usability Issues
- **Context Switching**: Confusing mix of reference data and procedures
- **Token Inefficiency**: Redundant information increases processing cost
- **Search Difficulty**: Hard to locate specific information quickly
- **Maintenance Burden**: Changes require updates in multiple places

## Recommended Split Structure

### 1. `accumulate-mainnet-reference.md` (Static Reference)
**Purpose**: Network specifications and configuration data
**Content**:
- Network architecture overview
- Port configurations
- Validator information
- Network configuration JSON examples
- Routing rules and special accounts

**AI Optimization**:
- Structured data tables
- Clear section headers with tags
- Minimal prose, maximum data density
- Quick reference format

### 2. `accumulated-daemon-commands.md` (Command Reference)
**Purpose**: Node daemon command documentation
**Content**:
- `accumulated init` subcommands
- `accumulated run` commands
- Flag documentation
- Configuration file structure
- Troubleshooting guide

**AI Optimization**:
- Command-first organization
- Consistent example format
- Error message catalog
- Cross-references to other docs

### 3. `cyclops-deployment-guide.md` (Operational Procedures)
**Purpose**: Deployment automation and procedures
**Content**:
- Manual deployment steps
- Automated script documentation
- Monitoring and troubleshooting
- Best practices

**AI Optimization**:
- Step-by-step procedures
- Clear prerequisites
- Expected outputs
- Troubleshooting decision trees

### 4. `accumulate-network-glossary.md` (Definitions)
**Purpose**: Terminology and concept definitions
**Content**:
- Technical terms (DN, BVN, ADI, etc.)
- Command definitions
- Configuration concepts
- Network topology terms

**AI Optimization**:
- Alphabetical organization
- Cross-references
- Context-specific definitions
- Usage examples

## Implementation Benefits

### For AI Processing
- **Focused Context**: Each document has single, clear purpose
- **Reduced Tokens**: Eliminate redundancy and irrelevant context
- **Better Search**: Targeted document selection based on query type
- **Faster Processing**: Smaller, focused documents load faster

### For Maintenance
- **Single Source of Truth**: Each fact documented once
- **Easier Updates**: Changes isolated to relevant document
- **Clear Ownership**: Each document has specific maintainer focus
- **Version Control**: Easier to track changes to specific areas

### For Users
- **Quick Reference**: Find information faster
- **Reduced Cognitive Load**: Less context switching
- **Better Navigation**: Clear document purpose and scope
- **Progressive Disclosure**: Start with overview, drill down as needed

## Immediate Actions Needed

1. **Extract Static Reference Data**: Move network specs to separate file
2. **Consolidate Command Documentation**: Remove redundant examples
3. **Standardize Heading Structure**: Consistent H1-H4 hierarchy
4. **Add Document Metadata**: Purpose, scope, and cross-references
5. **Create Index Document**: Overview with links to all related docs

## Success Metrics

- **Token Reduction**: 50%+ reduction in average query token usage
- **Search Accuracy**: Faster location of relevant information
- **Maintenance Efficiency**: Reduced time to update documentation
- **User Satisfaction**: Clearer, more focused documentation experience

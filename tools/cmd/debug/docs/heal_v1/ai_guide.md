# AI Assistant Guide to Healing Documentation

This guide is specifically designed to help AI assistants effectively navigate, understand, and utilize the healing documentation when assisting developers with debugging and implementation tasks. Human developers may also find this guide useful for understanding how to effectively prompt AI assistants when seeking help with healing-related tasks.

## How This Documentation is Organized for AI Consumption

The healing documentation is structured to be accessible to both human developers and AI assistants. This guide provides additional context and navigation aids specifically designed for AI assistants to overcome common limitations and optimize their assistance capabilities.

<!-- ai:context-priority
This section provides guidance on how to manage limited context windows when working with the healing codebase.
-->

## Context Management for AI Assistants

When working with the healing codebase, AI assistants often face context window limitations. Here's how to effectively manage your context:

### Priority Loading Order

When analyzing healing issues, load these files in the following order to maximize understanding with minimal context:

1. **Core Interfaces and Structures**:
   - `heal_common.go` - Contains shared interfaces and types
   - `healing/sequenced.go` - Contains the `SequencedInfo` structure used throughout

2. **Implementation Files** (based on the healing approach in question):
   - For anchor healing: `heal_anchor.go` and `healing/anchors.go`
   - For synthetic healing: `heal_synth.go` and `healing/synthetic.go`

3. **Supporting Components**:
   - For database issues: `database.md` and relevant database implementation files
   - For URL issues: `chains_urls.md` and URL construction code

### Context Chunking Strategy

Break your analysis into these manageable chunks:

1. **Interface Understanding**: First understand the interfaces and data structures
2. **Flow Analysis**: Analyze the control flow of the specific healing approach
3. **Component Deep Dive**: Only then examine specific components in detail

## Diagnostic Decision Trees for AI Assistants

<!-- ai:diagnostic-flow
This section provides structured decision trees for common healing issues.
-->

### "Element does not exist" Error Diagnosis

1. **Check URL Construction**
   - Is the code using `protocol.PartitionUrl(id)`?
     - Yes → Check if the partition ID is correct
     - No → Code may be using deprecated URL construction methods
   
2. **Verify Partition Querying**
   - Is the code querying the correct partition?
     - Check if source and destination partitions are correctly identified
   
3. **Examine Caching**
   - Is there a caching layer that might be returning stale data?
     - Check cache invalidation logic

### Transaction Submission Failures

1. **Signature Verification**
   - For anchor healing: Check signature collection logic
   - For synthetic healing: Verify receipt building process
   
2. **Version Compatibility**
   - Is the code using version-specific logic correctly?
     - Check `args.NetInfo.Status.ExecutorVersion` conditions
   
3. **Transaction Format**
   - Is the transaction formatted correctly for the target partition?
     - Verify transaction envelope structure

## Code Pattern Recognition Guide

<!-- ai:pattern-library
This section helps AI assistants recognize common patterns and anti-patterns in the healing codebase.
-->

### URL Construction Patterns

✅ **Correct Pattern**:
```go
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme
```

❌ **Incorrect Pattern**:
```go
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

### Transaction Creation Patterns

✅ **Correct Pattern**:
```go
// Check version first
if args.NetInfo.Status.ExecutorVersion.V2BaikonurEnabled() {
    // Use SyntheticMessage for Baikonur networks
    msg = &protocol.SyntheticMessage{...}
} else {
    // Use BadSyntheticMessage for pre-Baikonur networks
    msg = &protocol.BadSyntheticMessage{...}
}
```

❌ **Incorrect Pattern**:
```go
// Not checking version
msg = &protocol.SyntheticMessage{...}  // Will fail on pre-Baikonur networks
```

## Documentation Navigation for AI Assistants

<!-- ai:navigation-map
This section provides a map of the documentation specifically optimized for AI navigation.
-->

### Key Documentation Sections and Their Relationships

- **[Developer's Guide](./developer_guide.md)**: Start here for code-focused guidance
  - Links to: Implementation Guidelines, Transaction Creation, Database Requirements
  
- **[Healing Overview](./overview.md)**: Understand the two healing approaches
  - Links to: Chains and URLs, Implementation Guidelines
  
- **[Chains and URLs](./chains_urls.md)**: Critical for understanding URL construction
  - Referenced by: Developer's Guide, Implementation Guidelines
  
- **[Transaction Creation](./transactions.md)**: Details on creating healing transactions
  - Referenced by: Developer's Guide, Implementation Guidelines
  
- **[Database Requirements](./database.md)**: Database interfaces and implementations
  - Referenced by: Developer's Guide, Light Client Implementation
  
- **[Light Client Implementation](./light_client.md)**: Details on the Light client
  - Referenced by: Developer's Guide, Implementation Guidelines
  
- **[Implementation Guidelines](./implementation.md)**: Practical implementation advice
  - Referenced by: Developer's Guide, Healing Overview

### Documentation Dependency Graph

```
Developer's Guide
↑     ↑     ↑
|     |     |
Healing   Transaction   Implementation
Overview  Creation     Guidelines
   ↑         ↑            ↑
   |         |            |
Chains/URLs  Database   Light Client
```

## Effective Prompting Guidance for Humans

<!-- ai:prompting-guide
This section helps human developers effectively prompt AI assistants when seeking help with healing.
-->

When asking an AI assistant for help with healing-related tasks, structure your prompts to maximize effectiveness:

1. **Specify the Healing Approach**:
   - Indicate whether you're working with anchor healing or synthetic healing

2. **Provide Error Context**:
   - Include the full error message and stack trace if available
   - Mention which component is generating the error (database, URL resolution, etc.)

3. **Include Version Information**:
   - Specify if you're working with pre-Baikonur or Baikonur-enabled networks
   - Mention if you're using Version 1 or Version 2 healing implementations

4. **Reference Relevant Files**:
   - List the key files you're working with to help the AI load the right context

Example prompt:
```
I'm debugging an "element does not exist" error in the anchor healing process (heal_anchor.go).
The error occurs when querying a BVN partition with URL "acc://bvn-Apollo.acme".
This is on a Baikonur-enabled network using the Version 1 implementation.
The relevant files are heal_anchor.go and healing/anchors.go.
```

## See Also

- [Developer's Guide](./developer_guide.md)
- [Implementation Guidelines](./implementation.md)
- [Healing Overview](./overview.md)

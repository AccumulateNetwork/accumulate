# AI Assistant Guide to Version 2 Documentation

This guide is specifically designed to help AI assistants effectively navigate, understand, and utilize the Version 2 documentation when assisting developers with debugging and implementation tasks. Human developers may also find this guide useful for understanding how to effectively prompt AI assistants when seeking help with Version 2 healing-related tasks.

## How Version 2 Documentation Relates to Version 1

<!-- ai:context-priority
This section helps AI assistants understand the relationship between Version 1 and Version 2 documentation.
-->

Version 2 builds upon the foundation established in Version 1, with specific improvements and changes. When working with Version 2:

1. **Start with Version 2 documentation** for the specific components that have changed:
   - In-memory database implementation
   - URL construction standardization
   - Caching system
   - Fast sync optimization
   - Peer management

2. **Refer to Version 1 documentation** for foundational concepts that remain unchanged:
   - Basic healing approaches (anchor vs. synthetic)
   - Transaction creation fundamentals
   - Light client interfaces
   - Chain and URL concepts

3. **Compare Version 1 and Version 2 implementations** when debugging issues that might span both versions

## Context Management for AI Assistants

<!-- ai:context-management
This section provides guidance on how to manage limited context windows when working with the Version 2 codebase.
-->

When working with the Version 2 codebase, AI assistants often face context window limitations. Here's how to effectively manage your context:

### Priority Loading Order

When analyzing Version 2 issues, load these files in the following order to maximize understanding with minimal context:

1. **Core Components**:
   - `mem_db.md` - For in-memory database implementation details
   - `overview.md` - For understanding the key changes in Version 2

2. **Implementation-Specific Files** (based on the issue at hand):
   - For URL construction issues: Files containing URL construction code
   - For caching issues: Files implementing the caching system
   - For database issues: In-memory database implementation files

3. **Version 1 Reference** (as needed):
   - Load relevant Version 1 documentation to understand the foundation

### Context Chunking Strategy

Break your analysis into these manageable chunks:

1. **Change Assessment**: First understand what has changed from Version 1 to Version 2
2. **Implementation Analysis**: Analyze the specific implementation details of the changed components
3. **Integration Examination**: Examine how the changes integrate with unchanged components

## Diagnostic Decision Trees for AI Assistants

<!-- ai:diagnostic-flow
This section provides structured decision trees for common Version 2 issues.
-->

### "Element does not exist" Error Diagnosis

1. **Check URL Construction**
   - Is the code using the standardized `protocol.PartitionUrl(id)` approach?
     - Yes → Continue to step 2
     - No → Code needs to be updated to use the standardized approach
   
2. **Verify Caching System**
   - Is the caching system being used correctly?
     - Check cache key construction
     - Verify cache invalidation logic
   
3. **Examine Database Queries**
   - For in-memory database: Check if the data was properly initialized
     - Remember: In-memory database does not persist between restarts

### Performance Issue Diagnosis

1. **Database Lock Contention**
   - Are there multiple concurrent operations on the in-memory database?
     - Check read/write lock usage
     - Verify transaction isolation levels
   
2. **Caching Effectiveness**
   - Is the cache being utilized effectively?
     - Check cache hit rates
     - Verify cache key design
   
3. **Memory Usage Patterns**
   - Is memory usage growing unexpectedly?
     - Check for memory leaks in the in-memory database
     - Verify proper cleanup of resources

## Code Pattern Recognition Guide

<!-- ai:pattern-library
This section helps AI assistants recognize common patterns and anti-patterns in the Version 2 codebase.
-->

### URL Construction Patterns

✅ **Correct Pattern (Version 2)**:
```go
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme
```

❌ **Deprecated Pattern (Version 1)**:
```go
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

### Caching Implementation Patterns

✅ **Correct Pattern**:
```go
// Thread-safe cache access
func (c *cache) Get(url, typ string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    value, ok := c.cache[cacheKey{url, typ}]
    return value, ok
}
```

❌ **Incorrect Pattern**:
```go
// Not thread-safe
func (c *cache) Get(url, typ string) (interface{}, bool) {
    value, ok := c.cache[cacheKey{url, typ}]
    return value, ok
}
```

### In-Memory Database Patterns

✅ **Correct Pattern**:
```go
// Always defer discard to prevent resource leaks
batch := db.Begin(false)  // false = read-only
defer batch.Discard()

// Read operations
value, err := batch.Get(bucket, key)
```

❌ **Incorrect Pattern**:
```go
// No deferred discard, potential resource leak
batch := db.Begin(false)
value, err := batch.Get(bucket, key)
// Missing batch.Discard()
```

## Documentation Navigation for AI Assistants

<!-- ai:navigation-map
This section provides a map of the documentation specifically optimized for AI navigation.
-->

### Key Documentation Sections and Their Relationships

- **[Developer's Guide](./developer_guide.md)**: Start here for code-focused guidance on Version 2
  - Links to: Overview, MemDB Implementation, Fast Sync Implementation
  
- **[Overview](./overview.md)**: High-level overview of Version 2 changes
  - Links to: MemDB Implementation, Version 1 documentation
  
- **[MemDB Implementation](./mem_db.md)**: Details on the in-memory database
  - Referenced by: Developer's Guide, Overview
  
- **[Fast Sync Implementation](./fast_sync.md)**: Details on the fast sync optimization
  - Referenced by: Developer's Guide
  
- **[Peer Management](./peers.md)**: Information on improved peer management
  - Referenced by: Developer's Guide

### Version 1 References

- **[Version 1 Overview](../heal_v1/overview.md)**: Foundation for understanding healing
- **[Version 1 Implementation](../heal_v1/implementation.md)**: Core implementation details
- **[Version 1 Database](../heal_v1/database.md)**: Original database implementation
- **[Version 1 Chains and URLs](../heal_v1/chains_urls.md)**: Original URL construction

### Documentation Dependency Graph

```
Developer's Guide (V2)
↑     ↑     ↑
|     |     |
Overview   MemDB    Fast Sync
(V2)      Impl     Impl
   ↑         ↑        ↑
   |         |        |
   +----+----+--------+
        |
Version 1 Documentation
(Overview, Implementation, Database, etc.)
```

## Effective Prompting Guidance for Humans

<!-- ai:prompting-guide
This section helps human developers effectively prompt AI assistants when seeking help with Version 2.
-->

When asking an AI assistant for help with Version 2-related tasks, structure your prompts to maximize effectiveness:

1. **Specify Version 2**:
   - Clearly indicate you're working with Version 2 implementation

2. **Identify Changed Components**:
   - Mention which specific Version 2 component you're working with:
     - In-memory database
     - URL construction
     - Caching system
     - Fast sync
     - Peer management

3. **Provide Error Context**:
   - Include the full error message and stack trace if available
   - Mention which component is generating the error

4. **Reference Relevant Files**:
   - List the key files you're working with to help the AI load the right context

Example prompt:
```
I'm debugging a performance issue with the Version 2 in-memory database implementation.
The database operations are taking longer than expected when multiple concurrent 
transactions are running. I'm working with the MemDB implementation in mem_db.go.
```

## Comparing Version 1 and Version 2

<!-- ai:version-comparison
This section helps AI assistants understand the key differences between Version 1 and Version 2.
-->

| Component | Version 1 | Version 2 | Key Differences |
|-----------|-----------|-----------|----------------|
| Database | Bolt DB | In-memory DB | Non-persistent, faster access, no file I/O |
| URL Construction | Two approaches | Standardized | Consistent use of `protocol.PartitionUrl(id)` |
| Caching | Limited | Comprehensive | Thread-safe, tracks problematic nodes |
| Performance | Base implementation | Optimized | Reduced lock contention, optimized for memory |
| Peer Management | Basic | Enhanced | Improved reliability for cross-partition operations |

## See Also

- [Developer's Guide](./developer_guide.md)
- [Overview](./overview.md)
- [MemDB Implementation](./mem_db.md)
- [Version 1 AI Guide](../heal_v1/ai_guide.md)

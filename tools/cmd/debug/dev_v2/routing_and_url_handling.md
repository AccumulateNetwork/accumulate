# Routing and URL Handling in Healing

## Overview

This document provides information about the routing system in Accumulate and how it impacts the healing process. It explains the "cannot route message" errors that may occur during healing and references the detailed documentation available in the `heal_v1` directory.

## Routing in Accumulate

### Basic Concepts

The Accumulate network uses a routing system to determine which partition should process a particular message or transaction. Each URL in the system has a routing number that determines its destination partition.

### URL Formats

Accumulate uses different URL formats for different types of accounts and operations:

1. **Partition URLs**
   - Format: `acc://bvn-{PartitionID}.{domain}`
   - Example: `acc://bvn-Apollo.acme`

2. **Anchor Pool URLs**
   - Format: `acc://{domain}/anchors/{PartitionID}`
   - Example: `acc://dn.acme/anchors/Apollo`

### Impact on Healing

During the healing process, the system needs to submit messages to various partitions. If these messages contain URLs that would route to different partitions, a routing conflict occurs, resulting in the error:

```
cannot route request: cannot route message(s): conflicting routes
```

## URL Normalization

To prevent routing conflicts, the healing process should normalize URLs to a consistent format before submission. This ensures that all parts of a message route to the same partition.

### Implementation

The URL normalization approach is implemented in the submission process:

1. **Normalize URLs** before creating the envelope
2. **Handle routing conflicts** by applying more aggressive normalization if needed
3. **Cache successful submissions** to avoid redundant work

## Reference Documentation

For more detailed information about routing and URL handling, refer to the following documents in the `heal_v1` directory:

1. [**route.md**](../heal_v1/route.md) - Comprehensive documentation on the routing system
2. [**routing_errors.md**](../heal_v1/routing_errors.md) - Detailed explanation of routing error causes
3. [**implementation_plan.md**](../heal_v1/implementation_plan.md) - Plan for resolving routing issues
4. [**url_normalization.go**](../heal_v1/url_normalization.go) - Sample implementation of URL normalization
5. [**modified_submit_loop.go**](../heal_v1/modified_submit_loop.go) - Enhanced submission process with routing conflict handling

## Best Practices

When working with the healing process, follow these best practices to avoid routing conflicts:

1. **Use consistent URL construction** throughout your code
2. **Normalize URLs** before submission
3. **Handle routing conflicts** explicitly in error handling
4. **Cache successful submissions** to improve performance

## Integration with Healing Process

The healing process in `heal_all.go` relies on the submission logic in `heal_common.go`. The URL normalization and routing conflict handling should be integrated into the `submitLoop` function to ensure reliable message delivery.

### Key Components

1. **URL Normalization** - Convert between different URL formats to ensure consistent routing
2. **Enhanced Error Handling** - Specifically detect and handle routing conflicts
3. **Submission Caching** - Avoid redundant submissions of the same message

## Conclusion

Understanding the relationship between URL construction and routing is essential for a successful healing process. By implementing proper URL normalization and handling routing conflicts explicitly, we can ensure reliable message delivery across the Accumulate network.

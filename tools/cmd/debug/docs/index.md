# Debug Tools Documentation

This directory contains comprehensive documentation for the various debug tools and utilities in the Accumulate Network.

## Documentation Structure

The documentation is organized into the following sections:

### 1. [Peer Discovery](/tools/cmd/debug/docs/peer_discovery/index.md)
- Documentation for the peer discovery utility
- Extraction methods for host information from various address formats
- Testing and integration guidelines

### 2. [Code Examples](/tools/cmd/debug/docs/code_examples.md)
- Example code snippets for common debug tasks
- Usage patterns and best practices

### 3. Network Healing
- Documentation for the network healing process
- Anchor healing and chain validation

### 4. Address Directory
- Documentation for the AddressDir implementation
- Network peer management and discovery

## Implementation Guidelines

When implementing new features or utilities, please follow these documentation guidelines:

1. **Documentation Location**: 
   - Place all documentation in the appropriate subdirectory under `/tools/cmd/debug/docs/`
   - Create new subdirectories for major features or components

2. **Documentation Structure**:
   - Each subdirectory should have an `index.md` file
   - Use consistent formatting and structure across all documentation
   - Include examples, API references, and testing guidelines

3. **Cross-References**:
   - Use relative links to reference other documentation files
   - Update the main index when adding new documentation

4. **AI-Friendly Format**:
   - Include structured metadata where appropriate
   - Use consistent code block formatting
   - Provide comprehensive API references

## Related Code

The documentation in this directory corresponds to the code in the following locations:

- [Debug Tools](/tools/cmd/debug/)
- [Network Healing](/tools/cmd/debug/new_heal/)
- [Peer Discovery](/tools/cmd/debug/new_heal/peerdiscovery/)

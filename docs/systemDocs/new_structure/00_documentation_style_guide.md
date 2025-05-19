# Documentation Style Guide for Accumulate

## Metadata
- **Document Type**: Style Guide
- **Version**: 1.0
- **Last Updated**: 2025-05-17
- **Related Components**: Documentation System
- **Tags**: style_guide, documentation, standards

## 1. Introduction

This style guide establishes standards for the Accumulate documentation system. Following these guidelines ensures consistency, improves navigability, and enhances the experience for both human readers and AI systems processing the documentation.

## 2. File Structure and Organization

### 2.1 Directory Structure

Documentation is organized in a hierarchical structure:

```
new_structure/
├── 00_index.md
├── 00_documentation_style_guide.md
├── 01_user_guides/
├── 02_architecture/
├── 03_core_components/
├── 04_network/
├── 05_apis/
├── 06_implementation/
└── 07_operations/
```

### 2.2 File Naming Convention

All documentation files follow this naming pattern:

```
XX_descriptive_name.md
```

Where:
- `XX` is a two-digit number indicating the sequence within its directory
- `descriptive_name` is a lowercase, underscore-separated description

For sub-sections that require additional numbering:

```
XX_YY_descriptive_name.md
```

Where:
- `XX` is the main section number
- `YY` is the sub-section number

### 2.3 Index Files

Each major section should include an index file named `00_index.md` that:
- Provides an overview of the section
- Lists all documents in the section with brief descriptions
- Includes navigation links to all documents

## 3. Document Structure

### 3.1 Metadata Header

Each document should begin with a metadata header:

```markdown
# Document Title

## Metadata
- **Document Type**: [Technical/Conceptual/Guide/Reference]
- **Version**: [Version number]
- **Last Updated**: [YYYY-MM-DD]
- **Related Components**: [Components this document relates to]
- **Code References**: [Relevant code files or functions]
- **Tags**: [comma_separated, tags]
```

### 3.2 Section Numbering

Document sections should follow hierarchical numbering:

```markdown
# Main Title

## 1. First Main Section
### 1.1 Sub-section
### 1.2 Sub-section

## 2. Second Main Section
### 2.1 Sub-section
```

### 3.3 Cross-References

When referencing other documents, use relative links with descriptive text:

```markdown
See [Keybooks and Signatures](../../02_architecture/02_signatures/01_keybooks_and_signatures.md) for more information.
```

## 4. Content Guidelines

### 4.1 Code Examples

Code examples should:
- Include language specification
- Be properly indented
- Include comments for complex sections

```markdown
```go
// This function validates a signature
func ValidateSignature(sig *protocol.Signature) error {
    // Validation logic
    return nil
}
```
```

### 4.2 Diagrams

Diagrams should:
- Be stored in an `assets` directory
- Use SVG format when possible
- Include alt text for accessibility

### 4.3 AI Optimization

To optimize for AI processing:
- Keep individual files under 300-400 lines
- Use consistent terminology throughout
- Include specific sections for common AI queries
- Add code-to-documentation mappings

## 5. Document Types

### 5.1 Overview Documents

Overview documents provide high-level explanations and should:
- Begin with a concise introduction
- Explain core concepts
- Include diagrams where appropriate
- Link to more detailed documents

### 5.2 Implementation Documents

Implementation documents detail code-level specifics and should:
- Reference specific code files and functions
- Include relevant code snippets
- Explain design decisions
- Document edge cases and error handling

### 5.3 Tutorial Documents

Tutorial documents guide users through processes and should:
- Use step-by-step instructions
- Include command examples
- Show expected outputs
- Address common errors

## 6. Maintenance

### 6.1 Version Control

- Update the version number when making significant changes
- Update the "Last Updated" date for all changes
- Document major changes in a changelog section

### 6.2 Review Process

All documentation should undergo:
- Technical accuracy review
- Structural consistency review
- Language and clarity review

## Related Documents

- [Documentation Index](./00_index.md)
- [Getting Started](./01_user_guides/01_getting_started.md)

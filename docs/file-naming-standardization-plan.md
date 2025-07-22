# File Naming Standardization Plan

**Date**: 2025-01-20  
**Scope**: Standardize all markdown files in `/docs` to follow naming conventions

## Naming Convention Requirements

1. **Lowercase only** - no uppercase letters
2. **Dash-separated** - words separated by `-` (not `_` or camelCase)
3. **Descriptive** - clearly indicates content purpose

## Files Requiring Renaming

### 🚨 **Critical Violations (Uppercase/Underscore)**

| Current Name | New Name | Reason |
|--------------|----------|---------|
| `CONSOLIDATED-README.md` | `consolidated-readme.md` | Uppercase |
| `README.md` | `readme.md` | Uppercase |
| `CHANGELOG.md` | `changelog.md` | Uppercase |
| `CODE_OF_CONDUCT.md` | `code-of-conduct.md` | Uppercase + underscore |
| `CONTRIBUTING.md` | `contributing.md` | Uppercase |
| `Default.md` | `default.md` | Uppercase |
| `DOCUMENTATION_COMPLETE.md` | `documentation-complete.md` | Uppercase + underscore |
| `CYCLOPS_DEPLOYMENT_DESIGN.md` | `cyclops-deployment-design.md` | Uppercase + underscore |

### 🔧 **Underscore Violations**

| Current Name | New Name | Reason |
|--------------|----------|---------|
| `authority_validation.md` | `authority-validation.md` | Underscore |
| `lite_client.md` | `lite-client.md` | Underscore |
| `lite_client_test.md` | `lite-client-test.md` | Underscore |
| `a_extract_debug.md` | `a-extract-debug.md` | Underscore |
| `a_extract_debug_update.md` | `a-extract-debug-update.md` | Underscore |
| `a_extract_debug_update2.md` | `a-extract-debug-update-v2.md` | Underscore + better versioning |

### 📝 **Descriptiveness Improvements**

| Current Name | New Name | Reason |
|--------------|----------|---------|
| `readme.md` | `tools-overview.md` | More descriptive (in tools/) |
| `index.md` | `test-index.md` | More descriptive (in test/) |
| `system.md` | `protocol-system.md` | More descriptive (in protocol/) |
| `transactions.md` | `protocol-transactions.md` | More descriptive (in protocol/) |
| `debug.md` | `debug-tool-overview.md` | More descriptive |
| `analyze.md` | `analyze-tool-overview.md` | More descriptive |
| `simulator.md` | `simulator-tool-overview.md` | More descriptive |
| `factom.md` | `factom-tool-overview.md` | More descriptive |
| `testing.md` | `testing-overview.md` | More descriptive |
| `debugging.md` | `testing-debugging.md` | More descriptive |
| `snapshot.md` | `debug-snapshot-operations.md` | More descriptive (in debug/) |

### 📁 **Directory-Specific Improvements**

#### Root Directory
| Current | New | Reason |
|---------|-----|---------|
| `documentation-organization-summary.md` | `docs-organization-summary.md` | Shorter, clearer |
| `optimization-summary.md` | `docs-optimization-summary.md` | More specific |

#### API Directory
| Current | New | Reason |
|---------|-----|---------|
| `apiServer.md` | `api-server-reference.md` | Lowercase + descriptive |

#### Client Directory
| Current | New | Reason |
|---------|-----|---------|
| `lightclient-README.md` | `light-client-readme.md` | Consistent with other client docs |
| `database-README.md` | `database-readme.md` | Consistent naming |
| `api-v2-README.md` | `api-v2-readme.md` | Consistent naming |
| `database-smt-README.md` | `database-smt-readme.md` | Consistent naming |

#### Test Directory
| Current | New | Reason |
|---------|-----|---------|
| `testdata-index.md` | `test-data-index.md` | Dash separation |
| `testing-apiServer.md` | `testing-api-server.md` | Lowercase |
| `benchmark-README.md` | `benchmark-readme.md` | Consistent naming |

## Implementation Plan

### Phase 1: Critical Violations (Immediate)
Files with uppercase letters or underscores that break convention standards.

### Phase 2: Descriptiveness Improvements
Files that follow basic convention but could be more descriptive.

### Phase 3: Consistency Updates
Files that need minor adjustments for consistency across the documentation.

## Renaming Script Template

```bash
#!/bin/bash
# File renaming script for documentation standardization

# Phase 1: Critical violations
mv "CONSOLIDATED-README.md" "consolidated-readme.md"
mv "README.md" "readme.md"
mv "CHANGELOG.md" "changelog.md"
mv "CODE_OF_CONDUCT.md" "code-of-conduct.md"
mv "CONTRIBUTING.md" "contributing.md"
mv "Default.md" "default.md"
mv "DOCUMENTATION_COMPLETE.md" "documentation-complete.md"
mv "CYCLOPS_DEPLOYMENT_DESIGN.md" "cyclops-deployment-design.md"

# Phase 2: Underscore fixes
mv "authority_validation.md" "authority-validation.md"
mv "lite_client.md" "lite-client.md"
mv "lite_client_test.md" "lite-client-test.md"
mv "a_extract_debug.md" "a-extract-debug.md"
mv "a_extract_debug_update.md" "a-extract-debug-update.md"
mv "a_extract_debug_update2.md" "a-extract-debug-update-v2.md"

# Phase 3: Descriptiveness improvements
# (Additional renames based on context)
```

## Cross-Reference Updates Required

After renaming, the following files will need link updates:

1. **Main Index Files**:
   - `consolidated-readme.md` (main index)
   - Directory-specific readme files

2. **Cross-Referenced Documents**:
   - All files that link to renamed documents
   - Navigation sections in major documents

3. **Script References**:
   - Deployment scripts that reference documentation
   - Automation scripts with hardcoded paths

## Validation Checklist

- [ ] All files follow lowercase convention
- [ ] All files use dash separation (no underscores)
- [ ] File names are descriptive of content
- [ ] No broken internal links after renaming
- [ ] Index files updated with new names
- [ ] Cross-references updated throughout documentation
- [ ] Script references updated where applicable

## Benefits of Standardization

1. **AI Compatibility**: Consistent naming improves AI parsing and assistance
2. **Developer Experience**: Predictable file names improve navigation
3. **Automation**: Scripts can rely on consistent naming patterns
4. **Maintenance**: Easier to maintain and update documentation
5. **Professionalism**: Consistent naming reflects attention to detail

## Risk Mitigation

1. **Backup**: Create backup of entire `/docs` directory before renaming
2. **Staged Approach**: Implement in phases to minimize disruption
3. **Link Validation**: Run link checker after each phase
4. **Testing**: Test all cross-references and navigation after completion

---

*This plan ensures all documentation files follow consistent, AI-friendly naming conventions while maintaining content integrity and cross-reference accuracy.*

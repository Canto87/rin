# 0N_PHASE.md Template

Detailed design document for each phase.

---

```markdown
# Phase N: {Phase Name}

**Implementation Rank: N** | **Difficulty: {Low/Medium/High}** | **Impact: {Low/Medium/High}**

## Goal

{Core goal of this phase}

## Why This Rank?

{Reasoning for rank - dependencies, impact, etc.}

## Architecture

```
{Component diagram for this phase}
```

## Use Cases for This Phase

| ID | Use Case | Input | Output | Status |
|----|----------|-------|--------|--------|
| UC-{N}.1 | {use case name} | {input data} | {expected output} | Not implemented |
| UC-{N}.2 | {use case name} | {input data} | {expected output} | Not implemented |

## Interface Details

### Endpoints Implemented in This Phase

| Method | Endpoint | Request Body | Response | Notes |
|--------|----------|--------------|----------|-------|
| {METHOD} | `/api/{resource}` | `{schema}` | `{schema}` | {notes} |

### Request/Response Schema

**{Endpoint Name}**
```json
// Request
{
  "field1": "type: string, required",
  "field2": "type: number, optional"
}

// Success Response
{
  "data": {...},
  "message": "Success"
}

// Error Response
{
  "error": {
    "code": "{ERROR_CODE}",
    "message": "{message}"
  }
}
```

## Error Handling for This Phase

| Scenario | Error Code | HTTP Status | Handling |
|----------|------------|-------------|----------|
| {scenario1} | {CODE} | {status} | {action} |
| {scenario2} | {CODE} | {status} | {action} |

## Risk Assessment for This Phase

### Risk Level: {Low/Medium/High/Critical}

### Identified Risks

| Risk | Category | Impact | Probability | Level | Mitigation |
|------|----------|--------|-------------|-------|------------|
| {risk1} | {Technical/Dependency/Integration} | {High/Medium/Low} | {High/Medium/Low} | {Critical/High/Medium/Low} | {action} |
| {risk2} | {Technical/Dependency/Integration} | {High/Medium/Low} | {High/Medium/Low} | {Critical/High/Medium/Low} | {action} |

### Dependencies

| Dependency | Type | Status | Risk if Unavailable |
|------------|------|--------|---------------------|
| Phase {N-1} completion | Internal | {Required/Optional} | {impact} |
| {External service/API} | External | {Available/Pending} | {impact} |
| {Shared component} | Internal | {Stable/Changing} | {impact} |

### Rollback Plan

**Rollback Difficulty**: {Easy/Medium/Hard}

**Strategy**:
- {Step 1: e.g., Revert migration script}
- {Step 2: e.g., Restore from backup}
- {Step 3: e.g., Disable feature flag}

**Pre-Implementation Checklist**:
- [ ] Backup existing data/configuration
- [ ] Create feature flag for gradual rollout
- [ ] Prepare rollback script
- [ ] Notify dependent teams

## Implementation

### 1. {Component 1}

**File**: `{source_path}/{feature}/{file}.{ext}`

{Role description}

<!-- Write example code according to project language -->

**Go Example:**
```go
type {TypeName} struct {
    ...
}

func New{TypeName}() *{TypeName} {
    ...
}
```

**TypeScript Example:**
```typescript
export class {TypeName} {
    constructor() {
        ...
    }
}
```

**Python Example:**
```python
class {TypeName}:
    def __init__(self):
        ...
```

### 2. {Component 2}

**File**: `{source_path}/{feature}/{file2}.{ext}`

{Role description}

## Testing

<!-- Write commands according to project language/build system -->

```bash
# Build (by language)
# Go:         go build ./...
# TypeScript: npm run build
# Python:     python -m build
# Rust:       cargo build

# Run
{run_command}

# Test
# Go:         go test ./...
# TypeScript: npm test
# Python:     pytest
# Rust:       cargo test
```

## Checklist

- [ ] Create `{source_path}/{feature}/types.{ext}`
- [ ] Create `{source_path}/{feature}/store.{ext}`
- [ ] Write test code
- [ ] Integrate with existing systems
- [ ] Update documentation

## Next Steps

After completing this phase → Implement `0{N+1}_{NEXT}.md`

---

## Writing Guide

### Phase Division Criteria

1. **Dependencies**: Does another phase need to complete first?
2. **Difficulty**: Implementation complexity (Low/Medium/High)
3. **Impact**: User experience improvement (Low/Medium/High)
4. **Implementation Order**: Logical sequence

### Recommended Phase Count

- Minimum: 3
- Maximum: 7
- Ideal: 4-5

### Use Cases Writing Tips

- Extract relevant use cases from OVERVIEW for this phase
- Each phase should have 2-5 focused use cases
- Include input/output for each use case
- Mark status (Not implemented → In progress → Done)

### Interface Details Writing Tips

- Only include endpoints implemented in THIS phase
- Reference OVERVIEW for full API spec
- Include request/response examples
- Document validation rules

### Error Handling Writing Tips

- List errors specific to this phase's functionality
- Include both expected errors and edge cases
- Document recovery actions
- Reference OVERVIEW error codes

### Risk Assessment Writing Tips

- Identify top 3-5 risks for this specific phase
- Assess impact and probability realistically
- Include risks from:
  - Technical complexity (new technology, complex algorithms)
  - Dependencies (external APIs, shared components)
  - Integration (breaking changes, data migration)
- Define concrete rollback steps
- Consider:
  - Database changes → Hard to rollback, backup required
  - Config changes → Easy to rollback
  - Code changes → Medium, may need revert commit
- Mark dependencies clearly (required vs optional)
- Cross-reference with OVERVIEW risk assessment

### Checklist Writing Tips

- List in file creation order
- Include test items
- Specify integration tasks
- Include documentation updates
- Add interface/error handling verification

### Naming Conventions by Language

| Language | File Name | Type Name | Function Name |
|----------|-----------|-----------|---------------|
| Go | snake_case.go | PascalCase | PascalCase |
| TypeScript | kebab-case.ts | PascalCase | camelCase |
| Python | snake_case.py | PascalCase | snake_case |
| Rust | snake_case.rs | PascalCase | snake_case |

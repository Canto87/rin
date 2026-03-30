# Codebase Analysis Guide

Detailed guide for intelligent codebase analysis in Step 3.

---

## Analysis Overview

The goal is to understand the existing codebase to:
1. Identify reusable components
2. Detect architecture patterns
3. Recommend integration points
4. Avoid redundant implementations

---

## Step-by-Step Analysis Process

### 1. Initial Scan

```bash
# Get project structure
find {source_path} -type d -maxdepth 2

# Count files by type
find {source_path} -name "*.{ext}" | wc -l

# Read project config
cat CLAUDE.md 2>/dev/null || echo "No CLAUDE.md"
cat README.md 2>/dev/null | head -50
```

### 2. Architecture Detection

#### Go Projects

```bash
# Check for common patterns
ls -d {source}/domain 2>/dev/null && echo "Clean Architecture detected"
ls -d {source}/internal/service 2>/dev/null && echo "Layered detected"
ls -d {source}/pkg 2>/dev/null && echo "Pkg pattern detected"

# Find main packages
grep -r "^package main" {apps}/ --include="*.go"

# List all packages
go list ./...
```

#### TypeScript Projects

```bash
# Check structure
ls -d src/domain 2>/dev/null && echo "Clean Architecture"
ls -d src/controllers 2>/dev/null && echo "MVC pattern"
ls -d src/modules 2>/dev/null && echo "Modular pattern"

# Check framework
grep -l "express\|nestjs\|fastify" package.json
```

#### Python Projects

```bash
# Check structure
ls -d src/domain 2>/dev/null && echo "Clean Architecture"
ls -d app/models 2>/dev/null && echo "MVC pattern"

# Check framework
grep -l "django\|flask\|fastapi" requirements.txt
```

### 3. Component Discovery

#### Finding Types and Interfaces

| Language | Command | What to Find |
|----------|---------|--------------|
| Go | `grep -rn "type.*interface"` | Interfaces |
| Go | `grep -rn "type.*struct"` | Structs |
| TypeScript | `grep -rn "interface.*{"` | Interfaces |
| TypeScript | `grep -rn "class.*{"` | Classes |
| Python | `grep -rn "class.*:"` | Classes |
| Python | `grep -rn "@dataclass"` | Data classes |

#### Finding Services and Handlers

```bash
# Go - find service/handler patterns
grep -rln "func.*Service\|func.*Handler" {source}/

# TypeScript - find service classes
grep -rln "@Injectable\|class.*Service" {source}/

# Python - find service patterns
grep -rln "class.*Service\|def.*_service" {source}/
```

### 4. Dependency Mapping

#### Import Analysis

```bash
# Go - find imports
grep -rn "^import" {source}/ --include="*.go" | head -50

# TypeScript - find imports
grep -rn "^import\|require(" {source}/ --include="*.ts" | head -50

# Python - find imports
grep -rn "^from\|^import" {source}/ --include="*.py" | head -50
```

#### Build Dependency Graph

```
Module A
├── imports: Module B, Module C
├── exported: TypeX, FuncY
└── used_by: Module D

Module B
├── imports: Module C
├── exported: TypeZ
└── used_by: Module A, Module E
```

---

## Architecture Patterns Reference

### Clean Architecture

```
{source}/
├── domain/           # Entities, value objects
│   ├── entity/
│   └── repository/   # Repository interfaces
├── usecase/          # Business logic
│   ├── {feature}/
│   └── port/         # Input/Output ports
├── adapter/          # External implementations
│   ├── controller/   # HTTP handlers
│   ├── gateway/      # External services
│   └── repository/   # DB implementations
└── infrastructure/   # Framework code
    ├── db/
    └── http/
```

**Key indicators:**
- `domain/` contains pure business entities
- `usecase/` contains application logic
- `adapter/` contains implementations
- Dependencies point inward

### Layered Architecture

```
{source}/
├── api/              # HTTP handlers
│   └── handler/
├── service/          # Business logic
│   └── {feature}/
├── repository/       # Data access
│   └── {entity}/
└── model/            # Data models
    └── {entity}.go
```

**Key indicators:**
- Clear layer separation
- Each layer only calls the layer below
- Models shared across layers

### Modular Monolith

```
{source}/
├── modules/
│   ├── {module_a}/
│   │   ├── api/
│   │   ├── service/
│   │   └── repository/
│   └── {module_b}/
│       ├── api/
│       ├── service/
│       └── repository/
└── shared/
    ├── database/
    └── utils/
```

**Key indicators:**
- Each module is self-contained
- Shared utilities in common location
- Modules communicate via defined interfaces

### Feature-based (Go)

```
{source}/
├── {feature1}/
│   ├── handler.go
│   ├── service.go
│   ├── repository.go
│   └── types.go
├── {feature2}/
│   ├── handler.go
│   ├── service.go
│   └── types.go
└── shared/
    ├── database/
    └── middleware/
```

**Key indicators:**
- Features grouped in folders
- Each feature has similar file structure
- Shared code in common location

---

## Reusable Component Catalog

### High Reuse Potential

| Component | Typical Location | Signs of Reuse |
|-----------|-----------------|----------------|
| Database Client | `db/`, `database/`, `store/` | Generic interface, connection pooling |
| Logger | `log/`, `logger/` | Structured logging, multiple outputs |
| Config | `config/`, `cfg/` | Environment loading, validation |
| HTTP Client | `client/`, `httpclient/` | Retry logic, timeout handling |

### Medium Reuse Potential

| Component | Typical Location | Reuse Method |
|-----------|-----------------|--------------|
| Validators | `validator/`, `validate/` | Extend with new rules |
| Middleware | `middleware/` | Chain with new middleware |
| Error Handling | `errors/`, `apperror/` | Add new error types |
| Response Builder | `response/`, `render/` | Use for API responses |

### Reference Only

| Component | Typical Location | What to Copy |
|-----------|-----------------|--------------|
| Example Handler | `{feature}/handler.go` | Structure, patterns |
| Test Setup | `{feature}/*_test.go` | Test utilities |
| Makefile | `Makefile` | Build commands |

---

## Analysis Output Template

```markdown
## Codebase Analysis for {feature_name}

### Architecture Summary

**Detected Pattern**: {pattern_name}
**Confidence**: {High/Medium/Low}
**Language**: {language}
**Framework**: {framework_if_any}

### Directory Structure

\`\`\`
{source}/
├── {dir1}/     # {purpose}
├── {dir2}/     # {purpose}
└── {dir3}/     # {purpose}
\`\`\`

### Reusable Components

| Component | Location | Type | Action |
|-----------|----------|------|--------|
| {name} | {path} | {type} | {Direct/Extend/Reference} |

### Integration Points

For {feature_name}, integrate with:

1. **{component1}** at `{path1}`
   - Purpose: {why}
   - Method: {how}

2. **{component2}** at `{path2}`
   - Purpose: {why}
   - Method: {how}

### Recommended Structure

Based on existing patterns, create:

\`\`\`
{source}/{feature_name}/
├── {file1}     # {purpose}
├── {file2}     # {purpose}
└── {file3}     # {purpose}
\`\`\`

### Conventions to Follow

- Naming: {convention}
- File structure: {pattern}
- Error handling: {approach}
- Testing: {pattern}

### Potential Conflicts

- {existing_module} may need updates
- Consider {shared_resource} usage
- Watch for {potential_issue}

### Recommendations

1. {recommendation1}
2. {recommendation2}
3. {recommendation3}
```

---

## Integration with Phase Proposal

Analysis results affect phase planning:

| Finding | Impact on Phases |
|---------|-----------------|
| Many reusable components | Smaller Phase 1 (foundation) |
| Complex dependencies | More phases, careful ordering |
| Clean architecture | Follow existing layer structure |
| No clear pattern | Include architecture setup in Phase 1 |
| Existing similar feature | Reference for patterns, reduce scope |

### Example Impact

**Scenario**: Found existing `database/` and `auth/` modules

**Phase Adjustment**:
- Phase 1: Reduced - reuse database client
- Phase 2: Add auth integration step
- Overall: -1 phase from initial estimate

---

## Troubleshooting

### No Clear Architecture

If pattern is unclear:
1. Check for `CLAUDE.md` or `ARCHITECTURE.md`
2. Look at largest/most complex module
3. Ask user for clarification
4. Default to feature-based structure

### Empty or New Project

If codebase is minimal:
1. Skip reuse analysis
2. Recommend architecture based on project type
3. Include architecture setup in Phase 1
4. Suggest standard patterns for language

### Monorepo Detection

If multiple apps detected:
1. Identify which app to extend
2. Check for shared packages
3. Respect existing boundaries
4. Document cross-app dependencies

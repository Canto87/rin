# Phase Analysis Guide

How to analyze collected requirements and propose optimal phase structure.

---

## Analysis Input

Use information collected from Steps 2-5:

| Source | Data | Usage |
|--------|------|-------|
| Step 2 | Feature name, Core goal | Overall scope |
| Step 3 | Codebase analysis | Existing components, reuse opportunities |
| Step 4 | Integration, Storage, API | Technical complexity |
| Step 5 | Use cases, Interface, Errors, Security | Feature complexity |

---

## Complexity Scoring

### 1. Use Case Complexity

| Selection | Score | Reasoning |
|-----------|-------|-----------|
| CRUD only | 1 | Standard patterns, well-understood |
| CRUD + Data processing | 2 | Additional transformation logic |
| CRUD + User interaction | 2 | Forms, validation, feedback loops |
| All three | 3 | Full-featured, multiple concerns |

### 2. Integration Complexity

| Selection | Score | Reasoning |
|-----------|-------|-----------|
| None | 0 | Standalone module |
| Database only | 1 | Single integration point |
| Database + 1 external | 2 | Multiple integration points |
| 3+ integrations | 3 | Complex orchestration needed |

### 3. Interface Complexity

| Selection | Score | Reasoning |
|-----------|-------|-----------|
| Minimal | 0 | Internal use, flexible |
| Standard patterns | 1 | RESTful conventions |
| Detailed spec | 2 | Strict contracts, documentation |

### 4. Security Complexity

| Selection | Score | Reasoning |
|-----------|-------|-----------|
| None | 0 | Public access |
| Input validation only | 1 | Basic sanitization |
| Authentication | 2 | User identity required |
| Auth + Authorization | 3 | Role-based access control |

---

## Phase Count Formula

```
Base phases = 3

Additional phases:
  +1 if total_score >= 5
  +1 if total_score >= 8
  +1 if integrations >= 3
  +1 if security includes both auth + authz

Recommended = min(Base + Additional, 7)
```

### Score Ranges

| Total Score | Recommended Phases |
|-------------|-------------------|
| 0-4 | 3 phases |
| 5-7 | 4 phases |
| 8-10 | 5 phases |
| 11+ | 6-7 phases |

---

## Phase Structure Patterns

### Pattern A: Minimal (3 Phases)

For simple features (score 0-4):

```
Phase 1: Foundation
├── Data models
├── Storage setup
└── Basic types

Phase 2: Core Implementation
├── Main use case
├── API endpoints
└── Basic error handling

Phase 3: Polish
├── Edge cases
├── Validation refinement
└── Documentation
```

### Pattern B: Standard (4 Phases)

For moderate features (score 5-7):

```
Phase 1: Foundation
├── Data models
├── Storage layer
└── Configuration

Phase 2: Core Features
├── Primary use case (CRUD)
├── Main API endpoints
└── Basic validation

Phase 3: Extended Features
├── Secondary use cases
├── Integration with externals
└── Advanced queries

Phase 4: Hardening
├── Comprehensive error handling
├── Security implementation
└── Performance optimization
```

### Pattern C: Comprehensive (5-6 Phases)

For complex features (score 8+):

```
Phase 1: Foundation
├── Data models and types
├── Storage abstraction
└── Base configuration

Phase 2: Data Layer
├── Repository implementation
├── Query builders
└── Migrations

Phase 3: Core Business Logic
├── Primary use cases
├── Domain services
└── Validation rules

Phase 4: API Layer
├── REST endpoints
├── Request/Response handling
├── API documentation

Phase 5: Integration
├── External service clients
├── Event handling
└── Async processing

Phase 6: Security & Polish
├── Authentication
├── Authorization
├── Audit logging
└── Performance tuning
```

---

## Dependency Analysis

### Dependency Rules

1. **Foundation always first** - Data models, types, storage
2. **Core before extensions** - Primary use case before secondary
3. **Integration can parallel** - External integrations after foundation
4. **Security last or dedicated** - Can be final phase or separate

### Dependency Graph Examples

**Linear (Simple)**
```
Phase 1 → Phase 2 → Phase 3
```

**Branching (Moderate)**
```
Phase 1 → Phase 2 → Phase 4
              ↘ Phase 3 ↗
```

**Complex (Many integrations)**
```
Phase 1 → Phase 2 ──→ Phase 5
    ↘ Phase 3 ──────↗
    ↘ Phase 4 ──────↗
```

---

## Phase Content Assignment

### What Goes in Each Phase

| Phase Type | Must Include | May Include |
|------------|--------------|-------------|
| Foundation | Types, Storage interface | Config, Constants |
| Data Layer | Repository, Queries | Migrations, Seeds |
| Core Logic | Primary use case, Validation | Helper functions |
| API Layer | Endpoints, Handlers | Middleware, Docs |
| Integration | External clients | Retry logic, Circuit breaker |
| Security | Auth, Authz | Audit, Rate limiting |
| Polish | Error refinement | Performance, Monitoring |

### Use Case Distribution

```
If 1 use case type selected:
  → All in Phase 2 (Core)

If 2 use case types selected:
  → Primary in Phase 2
  → Secondary in Phase 3

If all 3 use case types:
  → CRUD in Phase 2
  → Data processing in Phase 3
  → User interaction in Phase 4
```

---

## Difficulty & Impact Estimation

### Difficulty Matrix

| Component | Low | Medium | High |
|-----------|-----|--------|------|
| CRUD operations | Standard DB | Complex queries | Distributed |
| Data processing | Simple transform | Aggregation | ML/Analytics |
| User interaction | Basic forms | Multi-step | Real-time |
| Integration | REST API call | OAuth/Webhook | Event streaming |
| Security | Basic auth | RBAC | Multi-tenant |

### Impact Matrix

| Phase | Impact Level | Reasoning |
|-------|--------------|-----------|
| Foundation | High | Blocks everything else |
| Core Features | High | Primary user value |
| Extensions | Medium | Additional value |
| Integration | Medium | External dependencies |
| Security | Medium-High | Risk mitigation |
| Polish | Low-Medium | Quality improvement |

---

## Adjustment Guidelines

### When User Wants Fewer Phases

**Merge candidates:**
1. Foundation + Core → Combined foundation
2. Extensions + Integration → Combined features
3. Security + Polish → Combined hardening

**Minimum viable: 2 phases**
- Phase 1: Foundation + Core
- Phase 2: Everything else

### When User Wants More Phases

**Split candidates:**
1. Each use case type → Separate phase
2. Each integration → Separate phase
3. Security → Dedicated phase
4. Testing → Dedicated phase

**Maximum recommended: 7 phases**
- Beyond 7: Consider splitting into multiple features

### When User Wants Custom Structure

**Validation checklist:**
- [ ] Foundation components in first phase?
- [ ] Dependencies respected?
- [ ] No circular dependencies?
- [ ] Each phase independently testable?
- [ ] Reasonable scope per phase?

---

## Example Analysis

### Input
```
Feature: user_auth
Core goal: Real-time processing
Integration: Database, External API (OAuth)
Storage: PostgreSQL
Use cases: CRUD, User interaction
Interface: Detailed spec
Security: Authentication, Authorization
```

### Scoring
```
Use cases: CRUD + User interaction = 2
Integration: DB + External = 2
Interface: Detailed = 2
Security: Auth + Authz = 3
Total: 9
```

### Recommendation
```
Score 9 → 5 phases recommended

Phase 1: Foundation (Medium/High)
  - User model, Session model
  - PostgreSQL setup
  - Configuration

Phase 2: Basic Auth (Medium/High)
  - Login/Logout
  - Password hashing
  - Session management

Phase 3: OAuth Integration (High/Medium)
  - OAuth provider setup
  - Token handling
  - Account linking

Phase 4: Authorization (Medium/High)
  - Role definitions
  - Permission checks
  - Middleware

Phase 5: Polish (Low/Medium)
  - Rate limiting
  - Audit logging
  - Error messages
```

---

## Output Template

```markdown
📋 Recommended Phase Structure

Based on your requirements:
- Use cases: {list} (complexity: {score})
- Integrations: {list} (complexity: {score})
- Interface: {level} (complexity: {score})
- Security: {list} (complexity: {score})
- Total complexity score: {total}

I recommend **{N} phases**:

{For each phase:}
┌──────────────────────────────────────────────┐
│ Phase {N}: {Name}                            │
│ Difficulty: {level} | Impact: {level}        │
├──────────────────────────────────────────────┤
│ • {Component 1}                              │
│ • {Component 2}                              │
│ • {Component 3}                              │
│ {Dependency or reasoning}                    │
└──────────────────────────────────────────────┘

Dependency Graph:
{ASCII dependency diagram}

Rationale:
- Phase 1 first because: {reason}
- Phase 2 depends on Phase 1 because: {reason}
- ...
```

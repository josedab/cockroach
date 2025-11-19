# RFC-0004: Developer Experience Improvements for Local Testing

**Status:** Draft
**Author:** Codebase Analysis
**Created:** November 19, 2025
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Summary

Improve the developer experience for CockroachDB contributors by reducing test iteration time and simplifying common development workflows.

## Motivation

### Current Pain Points

1. **Slow test discovery**: Finding which tests cover a change
2. **Long test runs**: Full package tests take minutes
3. **Complex commands**: Many flags to remember
4. **Limited feedback**: Test failures require investigation

### Impact

- Developer iteration cycles are slow
- New contributors face steep learning curve
- Less testing before commit

### Proposed Improvements

1. Intelligent test selection based on changed files
2. Faster test feedback with watch mode
3. Simplified commands for common tasks
4. Better failure diagnostics

## Detailed Design

### 1. Intelligent Test Selection

Automatically determine which tests to run based on changed files:

```bash
# Current: run all tests in package
./dev test pkg/sql -v

# Proposed: run only affected tests
./dev test --affected
# Analyzes git diff, finds affected tests
```

#### Implementation

```go
// pkg/cmd/dev/affected.go
func findAffectedTests(changedFiles []string) []string {
    var tests []string
    for _, file := range changedFiles {
        // Find tests that import this file's package
        // Find tests in the same package
        tests = append(tests, findTestsForFile(file)...)
    }
    return deduplicate(tests)
}
```

#### Dependency Graph

Build a dependency graph at startup:

```
pkg/sql/conn_executor.go
  └─► pkg/sql/conn_executor_test.go
  └─► pkg/sql/execinfra/...

pkg/kv/txn.go
  └─► pkg/kv/txn_test.go
  └─► pkg/sql/... (integration tests)
```

### 2. Watch Mode

Automatically re-run tests on file changes:

```bash
# Watch and re-run affected tests
./dev test --watch pkg/sql

# Watch specific test
./dev test --watch pkg/sql -f=TestConnExecutor
```

#### Implementation

```go
// Use fsnotify for file watching
watcher, _ := fsnotify.NewWatcher()
watcher.Add("pkg/sql")

for event := range watcher.Events {
    if event.Op&fsnotify.Write != 0 {
        runAffectedTests(event.Name)
    }
}
```

### 3. Simplified Commands

#### Quick Test

```bash
# Run tests for current changes (uncommitted)
./dev qt

# Equivalent to
./dev test --affected -v
```

#### Quick Build

```bash
# Build only affected packages
./dev qb

# Equivalent to
./dev build --affected
```

#### Quick Lint

```bash
# Lint only changed files
./dev ql

# Equivalent to
./dev lint --affected
```

### 4. Better Failure Diagnostics

#### Structured Test Output

```bash
./dev test pkg/sql -f=TestConnExecutor

─────────────────────────────────────────
FAIL: TestConnExecutor/transaction_retry
─────────────────────────────────────────

Location: pkg/sql/conn_executor_test.go:456

Expected: nil
Actual:   transaction retry error

Stack:
  conn_executor_test.go:456
  conn_executor.go:1234
  txn_coord_sender.go:567

Related logs:
  [conn_executor.go:1230] starting transaction
  [conn_executor.go:1234] ERROR: conflict detected

Suggestion: Check for concurrent writes to the same key
```

#### Flaky Test Detection

```bash
./dev test pkg/sql --stress --detect-flaky

TestConnExecutor/retry: FLAKY (3/10 failures)
  - Seeds: 12345, 67890, 11111
  - Pattern: Fails when concurrent ops > 5
```

### 5. Development Profiles

Pre-configured settings for common scenarios:

```bash
# Fast iteration (skip slow tests)
./dev test pkg/sql --profile=fast

# Thorough (include slow tests, race detection)
./dev test pkg/sql --profile=thorough

# CI simulation
./dev test pkg/sql --profile=ci
```

#### Profile Definition

```yaml
# .dev/profiles.yaml
profiles:
  fast:
    timeout: 1m
    skip_tags: [slow, integration]
    race: false

  thorough:
    timeout: 10m
    race: true
    stress: 5

  ci:
    timeout: 5m
    race: true
    verbose: true
```

### 6. Test Prioritization

Run likely-to-fail tests first:

```bash
./dev test pkg/sql --prioritize

# Order:
# 1. Tests for changed functions
# 2. Tests that failed recently
# 3. Tests with high code coverage of changes
# 4. Other tests
```

## Example Usage

### Typical Development Workflow

```bash
# Make changes
vim pkg/sql/conn_executor.go

# Quick test (just affected tests)
./dev qt
# ✓ TestConnExecutor (0.5s)
# ✓ TestTransactionRetry (0.3s)

# Watch mode while iterating
./dev test --watch pkg/sql -f=TestConnExecutor

# Before commit: thorough check
./dev test pkg/sql --profile=thorough
```

### New Contributor Workflow

```bash
# First time setup
./dev doctor

# Make a change
vim pkg/sql/parser/parse.go

# What tests should I run?
./dev test --affected --list
# pkg/sql/parser/parse_test.go
# pkg/sql/parser/parser_test.go
# pkg/sql/opt/optbuilder/builder_test.go

# Run them
./dev qt
```

## Implementation Plan

### Phase 1: Affected Test Selection (4 days)
- [ ] Build dependency graph
- [ ] Implement `--affected` flag
- [ ] Add `./dev qt` shortcut

### Phase 2: Watch Mode (3 days)
- [ ] File watching with fsnotify
- [ ] Debouncing and filtering
- [ ] Clear re-run output

### Phase 3: Better Diagnostics (2 days)
- [ ] Structured failure output
- [ ] Related log extraction
- [ ] Suggestion system

### Phase 4: Profiles and Prioritization (1 day)
- [ ] Profile configuration
- [ ] Test prioritization

## Backwards Compatibility

- All existing commands continue to work
- New features are additive
- Profiles are optional

## Alternatives Considered

### 1. IDE Integration Only

**Pros**: Rich UI
**Cons**: Not everyone uses same IDE
**Decision**: CLI-first, IDE plugins later

### 2. Separate Tool

**Pros**: Independence
**Cons**: Another tool to maintain
**Decision**: Extend existing `./dev`

## Open Questions

1. **Dependency accuracy**: How accurate is static analysis?
2. **Watch performance**: Impact on file system?
3. **Cache invalidation**: When to rebuild dependency graph?

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| Test iteration time | -50% | Time from edit to result |
| Commands to remember | -70% | Developer survey |
| Time to first contribution | -30% | Contributor tracking |
| Developer satisfaction | +25% | Survey |

## Effort Estimation

- **Total**: 10 dev-days
- **Affected selection**: 4 days
- **Watch mode**: 3 days
- **Diagnostics**: 2 days
- **Profiles**: 1 day

## Required Approvals

- [ ] Developer Experience Lead
- [ ] Build Team

## References

- Go test caching
- Bazel remote caching
- Jest watch mode

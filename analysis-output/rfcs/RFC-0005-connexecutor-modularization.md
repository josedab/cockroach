# RFC-0005: Modularize ConnExecutor for Maintainability

**Status:** Draft
**Author:** Codebase Analysis
**Created:** November 19, 2025
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Summary

Refactor the `connExecutor` struct into smaller, focused modules to improve maintainability, testability, and developer comprehension.

## Motivation

### Current State

The `connExecutor` in [`pkg/sql/conn_executor.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/conn_executor.go) is approximately 3,000 lines and handles:

- Statement execution
- Transaction management
- Session state
- Memory accounting
- Metrics collection
- Cursor management
- Automatic retry
- Result delivery

### Problems

1. **Difficult to understand**: New contributors struggle
2. **Hard to test**: Integration testing required
3. **Risk of bugs**: Changes have wide impact
4. **Slow reviews**: Reviewers must understand all concerns

### Goal

Break down into focused modules that can be:
- Understood independently
- Tested in isolation
- Modified with confidence

## Detailed Design

### Proposed Module Structure

```
pkg/sql/
├── conn_executor.go          # Coordinator (500 lines)
├── exec/
│   ├── statement.go          # Statement execution
│   ├── transaction.go        # Transaction management
│   ├── session.go            # Session state
│   ├── memory.go             # Memory accounting
│   ├── metrics.go            # Metrics collection
│   ├── cursor.go             # Cursor management
│   └── retry.go              # Automatic retry
```

### Module Responsibilities

#### 1. Coordinator (`conn_executor.go`)

Orchestrates the other modules:

```go
type connExecutor struct {
    stmtExec    *exec.StatementExecutor
    txnMgr      *exec.TransactionManager
    sessionMgr  *exec.SessionManager
    memMgr      *exec.MemoryManager
    metricsMgr  *exec.MetricsManager
    cursorMgr   *exec.CursorManager
    retryMgr    *exec.RetryManager
}

func (e *connExecutor) execStmt(ctx context.Context, stmt Statement) error {
    // Coordinate between modules
    e.memMgr.BeginStatement()
    defer e.memMgr.EndStatement()

    return e.stmtExec.Execute(ctx, stmt, e.txnMgr, e.sessionMgr)
}
```

#### 2. Statement Executor (`exec/statement.go`)

Handles statement parsing and execution:

```go
type StatementExecutor struct {
    parser    *parser.Parser
    optimizer *xform.Optimizer
    execFac   exec.Factory
}

func (s *StatementExecutor) Execute(
    ctx context.Context,
    stmt Statement,
    txn TransactionContext,
    session SessionContext,
) error {
    ast, err := s.parser.Parse(stmt.SQL)
    if err != nil {
        return err
    }

    plan, err := s.optimizer.Optimize(ctx, ast, txn, session)
    if err != nil {
        return err
    }

    return s.execFac.Execute(ctx, plan)
}
```

#### 3. Transaction Manager (`exec/transaction.go`)

Manages transaction lifecycle:

```go
type TransactionManager struct {
    txn      *kv.Txn
    state    txnState
    implicit bool
}

func (t *TransactionManager) Begin(ctx context.Context) error
func (t *TransactionManager) Commit(ctx context.Context) error
func (t *TransactionManager) Rollback(ctx context.Context) error
func (t *TransactionManager) Savepoint(name string) error
func (t *TransactionManager) RollbackToSavepoint(name string) error
```

#### 4. Session Manager (`exec/session.go`)

Manages session state:

```go
type SessionManager struct {
    data      *sessiondata.SessionData
    prepared  map[string]*PreparedStatement
    settings  map[string]string
}

func (s *SessionManager) Set(key, value string) error
func (s *SessionManager) Get(key string) string
func (s *SessionManager) Prepare(name, sql string) error
func (s *SessionManager) Deallocate(name string) error
```

#### 5. Memory Manager (`exec/memory.go`)

Handles memory accounting:

```go
type MemoryManager struct {
    sessionMon *mon.BytesMonitor
    txnMon     *mon.BytesMonitor
    stmtMon    *mon.BytesMonitor
}

func (m *MemoryManager) BeginStatement()
func (m *MemoryManager) EndStatement()
func (m *MemoryManager) Allocate(bytes int64) error
func (m *MemoryManager) Release(bytes int64)
```

#### 6. Retry Manager (`exec/retry.go`)

Handles automatic transaction retry:

```go
type RetryManager struct {
    buffer    *StmtBuf
    canRetry  bool
    retryErr  error
}

func (r *RetryManager) RecordStatement(stmt Statement)
func (r *RetryManager) CanRetry(err error) bool
func (r *RetryManager) GetRetryStatements() []Statement
```

### Interface Contracts

Define clear interfaces between modules:

```go
// TransactionContext provides transaction info to other modules
type TransactionContext interface {
    Txn() *kv.Txn
    IsolationLevel() tree.IsolationLevel
    ReadOnly() bool
}

// SessionContext provides session info to other modules
type SessionContext interface {
    User() username.SQLUsername
    Database() string
    SearchPath() sessiondata.SearchPath
    Settings() map[string]string
}
```

### Testing Strategy

Each module is independently testable:

```go
func TestStatementExecutor_Select(t *testing.T) {
    // Create mock transaction and session contexts
    txnCtx := &mockTransactionContext{...}
    sessCtx := &mockSessionContext{...}

    exec := NewStatementExecutor(...)
    err := exec.Execute(ctx, stmt, txnCtx, sessCtx)

    require.NoError(t, err)
}
```

### Migration Path

#### Phase 1: Extract with Delegation

Extract modules but keep original code:

```go
// Original method delegates to new module
func (e *connExecutor) execStmt(...) error {
    return e.stmtExec.Execute(...)
}
```

#### Phase 2: Remove Delegation

Once stable, remove original code.

#### Phase 3: Optimize Interfaces

Refine interfaces based on usage patterns.

## Example Usage

### Before Refactor

```go
// Understanding conn_executor requires reading 3000 lines
func (e *connExecutor) execStmt(ctx context.Context, ...) error {
    // Memory accounting
    // Transaction management
    // Statement execution
    // Metrics recording
    // Retry handling
    // ... 200+ lines
}
```

### After Refactor

```go
// Clear responsibilities, 10-20 lines
func (e *connExecutor) execStmt(ctx context.Context, stmt Statement) error {
    e.memMgr.BeginStatement()
    defer e.memMgr.EndStatement()

    if err := e.stmtExec.Execute(ctx, stmt, e.txnMgr, e.sessionMgr); err != nil {
        if e.retryMgr.CanRetry(err) {
            return e.retryTransaction(ctx)
        }
        return err
    }

    e.metricsMgr.RecordSuccess(stmt)
    return nil
}
```

## Implementation Plan

### Phase 1: Design and Interfaces (1 week)
- [ ] Finalize module boundaries
- [ ] Define interfaces
- [ ] Design test strategy

### Phase 2: Extract Modules (3 weeks)
- [ ] Week 1: Session and Memory managers
- [ ] Week 2: Transaction and Retry managers
- [ ] Week 3: Statement executor and Metrics

### Phase 3: Integration and Testing (1 week)
- [ ] Integration tests
- [ ] Performance validation
- [ ] Documentation

### Phase 4: Cleanup (1 week)
- [ ] Remove delegation
- [ ] Code review
- [ ] Final testing

## Backwards Compatibility

- **No API changes**: External interfaces unchanged
- **No behavior changes**: Same functionality
- **Internal only**: Refactor is internal

## Alternatives Considered

### 1. Partial Extraction

**Pros**: Less risky
**Cons**: Doesn't fully address problem
**Decision**: Full modularization needed

### 2. Complete Rewrite

**Pros**: Clean slate
**Cons**: High risk, long timeline
**Decision**: Too risky

## Open Questions

1. **Module boundaries**: Are these the right divisions?
2. **Shared state**: How to handle shared state between modules?
3. **Performance**: Any overhead from indirection?
4. **Error handling**: Consistent error propagation?

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| Code review time | -30% | PR metrics |
| New contributor ramp-up | -40% | Survey |
| Test coverage per module | >80% | Coverage tools |
| Bug rate in conn_executor | -25% | Issue tracking |

## Effort Estimation

- **Total**: 40 dev-days (8 weeks)
- **Design**: 5 days
- **Extraction**: 25 days
- **Testing**: 7 days
- **Cleanup**: 3 days

## Required Approvals

- [ ] SQL Team Lead
- [ ] Architecture Review
- [ ] Performance Team

## Rollback Strategy

Each phase is independently deployable. If issues arise:
1. Revert to previous phase
2. Fix issues
3. Re-deploy

## References

- Clean Architecture (Martin)
- Refactoring (Fowler)
- CockroachDB coding guidelines

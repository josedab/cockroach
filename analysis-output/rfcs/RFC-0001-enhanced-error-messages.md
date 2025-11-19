# RFC-0001: Enhanced Error Messages with Troubleshooting Guidance

**Status:** Draft
**Author:** Codebase Analysis
**Created:** November 19, 2025
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Summary

Enhance CockroachDB error messages to include actionable troubleshooting guidance, reducing time-to-resolution for common issues and decreasing support ticket volume.

## Motivation

### Current State

CockroachDB error messages are technically accurate but often lack actionable guidance:

```
ERROR: transaction deadline exceeded
SQLSTATE: 40001
```

Users must:
1. Search documentation
2. Review logs
3. Possibly contact support

### Proposed Improvement

```
ERROR: transaction deadline exceeded
SQLSTATE: 40001
HINT: This transaction took longer than the configured timeout.
  - Check for long-running statements with: SELECT * FROM crdb_internal.cluster_queries;
  - Review contention: SELECT * FROM crdb_internal.cluster_contention_events;
  - Increase timeout: SET statement_timeout = '5m';
DOCS: https://cockroachlabs.com/docs/stable/transaction-retry-error-reference.html#40001
```

### Problems Solved

1. **Reduced support tickets**: Common issues become self-serviceable
2. **Faster debugging**: Immediate next steps
3. **Better documentation discovery**: Direct links to relevant docs
4. **Improved DX**: Developers stay in their workflow

## Detailed Design

### Error Enhancement Structure

```go
// pkg/sql/pgwire/pgerror/errors.go
type EnhancedError struct {
    // Existing fields
    Code    pgcode.Code
    Message string

    // New fields
    Hint          string   // Troubleshooting steps
    Detail        string   // Additional context
    DocLink       string   // Documentation URL
    RelatedErrors []string // Related error codes
}
```

### Implementation Locations

1. **Transaction errors**: `pkg/kv/kvpb/errors.go`
2. **SQL errors**: `pkg/sql/pgwire/pgerror/`
3. **Schema errors**: `pkg/sql/catalog/`

### Error Categories to Enhance

| Category | Example | Enhancement |
|----------|---------|-------------|
| Transaction retry | 40001 | Contention queries, timeout adjustment |
| Timeout | 57014 | Timeout settings, query optimization |
| Connection | 08000 | Network diagnostics, pool settings |
| Permission | 42501 | Grant syntax, role checks |
| Schema | 42P01 | Object existence checks |

### Example Enhancements

#### Transaction Retry Error

**Before:**
```
ERROR: restart transaction: TransactionRetryWithProtoRefreshError
```

**After:**
```
ERROR: restart transaction: TransactionRetryWithProtoRefreshError
HINT: Transaction conflicted with another and must be retried.
  Common causes:
  - High contention on hot keys
  - Long-running transactions

  Troubleshooting:
  - View contention: SELECT * FROM crdb_internal.cluster_contention_events;
  - Check transaction duration: SELECT * FROM crdb_internal.cluster_queries WHERE txn_id = '...';
  - Consider adding retry logic: https://cockroachlabs.com/docs/stable/transactions.html#client-side-intervention

DOCS: https://cockroachlabs.com/docs/stable/common-errors.html#restart-transaction
```

#### Out of Memory

**Before:**
```
ERROR: memory budget exceeded
```

**After:**
```
ERROR: memory budget exceeded (requested 128 MiB, limit 512 MiB)
HINT: Query exceeded memory allocation.

  Immediate actions:
  - Reduce result set size with LIMIT
  - Add index to avoid full table scan
  - Increase memory budget (if available)

  Diagnostics:
  - Check current memory: SELECT * FROM crdb_internal.node_memory_monitors;
  - View query stats: SELECT * FROM crdb_internal.cluster_queries WHERE status = 'running';

DOCS: https://cockroachlabs.com/docs/stable/configure-zone.html#memory-limits
```

### Hint Registry

Create a central registry for hints:

```go
// pkg/sql/pgwire/pgerror/hints.go
var hintRegistry = map[pgcode.Code]HintTemplate{
    pgcode.SerializationFailure: {
        Template: `Transaction conflicted with another.
  - View contention: {{.ContentionQuery}}
  - Check locks: {{.LockQuery}}`,
        DocLink: "https://cockroachlabs.com/docs/stable/common-errors.html#40001",
    },
    // ...
}
```

### Dynamic Context

Some hints include dynamic context:

```go
func (e *Error) WithContext(ctx HintContext) *Error {
    e.Hint = e.hintTemplate.Execute(ctx)
    return e
}

// Usage
return pgerror.New(pgcode.SerializationFailure, "restart transaction").
    WithContext(HintContext{
        ContentionQuery: "SELECT * FROM crdb_internal.cluster_contention_events LIMIT 10",
        TxnID:           txn.ID(),
    })
```

## Example Usage

### Application Code

```go
_, err := db.Exec("INSERT INTO orders ...")
if err != nil {
    // Error now includes hints
    fmt.Println(err)
    // ERROR: transaction deadline exceeded
    // HINT: ...
}
```

### psql Output

```sql
mydb=> SELECT * FROM large_table;
ERROR:  memory budget exceeded
HINT:  Query exceeded memory allocation.
  - Reduce result set size with LIMIT
  - Add index to avoid full table scan
  - Increase memory budget: SET distsql_workmem = '256MB'
```

## Implementation Plan

### Phase 1: Infrastructure (3 days)
- [ ] Define `EnhancedError` structure
- [ ] Create hint registry
- [ ] Update error formatting

### Phase 2: High-Impact Errors (5 days)
- [ ] Transaction retry errors (40001)
- [ ] Timeout errors (57014)
- [ ] Memory errors
- [ ] Permission errors (42501)

### Phase 3: Comprehensive Coverage (Ongoing)
- [ ] Remaining error codes
- [ ] Review based on support tickets

## Backwards Compatibility

- **Wire protocol**: No changes (hints in NOTICE field)
- **Error codes**: Unchanged
- **Log format**: New fields added
- **Client libraries**: Automatically receive enhanced errors

## Alternatives Considered

### 1. External Error Lookup Service

**Pros**: Hints can be updated without release
**Cons**: Network dependency, latency
**Decision**: Rejected due to complexity

### 2. Documentation-Only Improvements

**Pros**: No code changes
**Cons**: Users must leave their workflow
**Decision**: Complement, not replace

### 3. AI-Powered Suggestions

**Pros**: Context-aware hints
**Cons**: Complexity, non-deterministic
**Decision**: Future consideration

## Open Questions

1. **Hint verbosity**: Should hints be configurable (verbose/brief)?
2. **Localization**: Should hints be translatable?
3. **Telemetry**: Track which hints are displayed?
4. **Client display**: How should clients format hints?

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| Support tickets for covered errors | -20% | Zendesk analytics |
| Documentation page views from hints | +30% | Analytics |
| Time-to-resolution for common errors | -40% | Customer feedback |
| Developer satisfaction | +15% | Survey |

## Effort Estimation

- **Total**: 3 dev-days
- **Infrastructure**: 1 day
- **High-impact errors**: 2 days
- **Testing**: Included

## Required Approvals

- [ ] SQL Team Lead
- [ ] Documentation Team
- [ ] Customer Success (hint content review)

## Rollback Strategy

Hints can be disabled via cluster setting:

```sql
SET CLUSTER SETTING sql.errors.enhanced_hints.enabled = false;
```

## References

- PostgreSQL Error Message Style Guide
- CockroachDB Error Handling RFC
- Support ticket analysis (internal)

---

## Appendix: Error Code Coverage

### Phase 2 Targets

| Code | Name | Frequency |
|------|------|-----------|
| 40001 | serialization_failure | High |
| 57014 | query_canceled | High |
| 08000 | connection_exception | Medium |
| 42501 | insufficient_privilege | Medium |
| 42P01 | undefined_table | Medium |
| 53400 | configuration_limit_exceeded | Medium |

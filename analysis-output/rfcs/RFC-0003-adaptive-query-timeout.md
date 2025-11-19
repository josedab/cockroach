# RFC-0003: Adaptive Query Timeout Based on Historical Execution

**Status:** Draft
**Author:** Codebase Analysis
**Created:** November 19, 2025
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Summary

Implement adaptive query timeouts that automatically adjust based on historical execution patterns, reducing timeout-related failures while maintaining protection against runaway queries.

## Motivation

### Current State

CockroachDB uses static timeouts:
```sql
SET statement_timeout = '30s';
```

Problems:
1. **One size doesn't fit all**: Simple queries and complex reports need different timeouts
2. **Guessing game**: Operators must guess appropriate values
3. **Timeout failures**: Legitimate queries fail when data grows
4. **Over-provisioning**: High timeouts allow runaway queries

### Proposed Solution

Adaptive timeouts that:
1. Learn from historical execution
2. Set per-query timeouts based on fingerprint
3. Apply multipliers for safety margin
4. Fall back to static timeout when no history exists

## Detailed Design

### Query Fingerprint

Use query fingerprint (normalized query without literals):

```sql
-- These share a fingerprint
SELECT * FROM users WHERE id = 5
SELECT * FROM users WHERE id = 12345

-- Fingerprint
SELECT * FROM users WHERE id = _
```

**Location**: Leverages existing fingerprint in `pkg/sql/sqlstats/`.

### Historical Statistics

Track execution history per fingerprint:

```go
type QueryStats struct {
    Fingerprint    string
    ExecutionCount int64
    LatencyP50     time.Duration
    LatencyP99     time.Duration
    LatencyMax     time.Duration
    LastUpdated    time.Time
}
```

**Storage**: Extend `crdb_internal.statement_statistics`.

### Timeout Calculation

```go
func calculateTimeout(stats QueryStats, config AdaptiveConfig) time.Duration {
    if stats.ExecutionCount < config.MinSamples {
        return config.DefaultTimeout
    }

    // Use P99 with multiplier for safety
    adaptive := stats.LatencyP99 * config.Multiplier

    // Clamp to configured bounds
    if adaptive < config.MinTimeout {
        return config.MinTimeout
    }
    if adaptive > config.MaxTimeout {
        return config.MaxTimeout
    }

    return adaptive
}
```

### Configuration

```sql
-- Enable adaptive timeouts
SET CLUSTER SETTING sql.adaptive_timeout.enabled = true;

-- Configure multiplier (default 2.0)
SET CLUSTER SETTING sql.adaptive_timeout.multiplier = 2.0;

-- Minimum samples before using adaptive (default 10)
SET CLUSTER SETTING sql.adaptive_timeout.min_samples = 10;

-- Bounds
SET CLUSTER SETTING sql.adaptive_timeout.min = '1s';
SET CLUSTER SETTING sql.adaptive_timeout.max = '5m';
SET CLUSTER SETTING sql.adaptive_timeout.default = '30s';
```

### Session Override

```sql
-- Disable for this session
SET adaptive_timeout = off;

-- Force specific timeout (overrides adaptive)
SET statement_timeout = '60s';
```

### Architecture

```
Query Execution
       │
       ▼
┌──────────────┐
│ Get Fingerprint │
└──────┬───────┘
       │
       ▼
┌──────────────┐     ┌─────────────┐
│ Lookup Stats │◄────┤ Stats Cache │
└──────┬───────┘     └─────────────┘
       │
       ▼
┌──────────────┐
│ Calculate    │
│ Timeout      │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Execute with │
│ Adaptive TO  │
└──────┬───────┘
       │
       ▼
┌──────────────┐     ┌─────────────┐
│ Update Stats │────►│ Stats Cache │
└──────────────┘     └─────────────┘
```

### Stats Collection

```go
// After query execution
func (e *connExecutor) recordQueryStats(fingerprint string, latency time.Duration) {
    e.statsCollector.RecordLatency(fingerprint, latency)
}

// Periodic aggregation
func (c *StatsCollector) aggregate() {
    // Compute P50, P99, update storage
}
```

## Example Usage

### Normal Operation

```sql
-- First executions use default timeout
SELECT * FROM orders WHERE user_id = 5;
-- Timeout: 30s (default)

-- After 10 executions, adaptive kicks in
-- Historical P99: 200ms
SELECT * FROM orders WHERE user_id = 6;
-- Timeout: 400ms (200ms × 2.0 multiplier)
```

### Slow Query Protection

```sql
-- Query slows down due to data growth
-- Historical P99: 200ms → 2s
SELECT * FROM orders WHERE user_id = 7;
-- Timeout: 4s (2s × 2.0)
-- Adapts automatically
```

### Monitoring

```sql
-- View adaptive timeouts
SELECT
    fingerprint_id,
    metadata->>'query' AS query,
    statistics->'latencyP99' AS p99_latency,
    (statistics->'latencyP99')::FLOAT * 2.0 AS adaptive_timeout
FROM crdb_internal.statement_statistics
WHERE statistics->'count' > 10;
```

## Implementation Plan

### Phase 1: Statistics Extension (5 days)
- [ ] Extend statement statistics with latency percentiles
- [ ] Implement aggregation logic
- [ ] Add caching layer

### Phase 2: Timeout Calculation (5 days)
- [ ] Implement adaptive timeout calculation
- [ ] Integrate with query execution
- [ ] Add cluster settings

### Phase 3: Observability (3 days)
- [ ] Add metrics for adaptive timeouts
- [ ] Logging for timeout decisions
- [ ] Virtual table for monitoring

### Phase 4: Testing (2 days)
- [ ] Unit tests
- [ ] Integration tests
- [ ] Performance validation

## Backwards Compatibility

- **Default off**: No change unless enabled
- **Existing settings honored**: `statement_timeout` overrides
- **Gradual rollout**: Can enable per-session first

## Alternatives Considered

### 1. Machine Learning Prediction

**Pros**: More accurate predictions
**Cons**: Complexity, explainability
**Decision**: Start simple, enhance later

### 2. Query-Plan Based Timeout

**Pros**: Uses execution plan
**Cons**: Doesn't account for data skew
**Decision**: Fingerprint is simpler and accounts for actual execution

### 3. Per-Table Statistics

**Pros**: Simpler
**Cons**: Doesn't distinguish queries on same table
**Decision**: Fingerprint is more accurate

## Open Questions

1. **Cold start**: How long until adaptive is useful?
2. **Outliers**: How to handle occasional slow executions?
3. **Schema changes**: Reset stats on schema change?
4. **Tenant isolation**: Separate stats per tenant?

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| Timeout-related failures | -50% | Error metrics |
| False timeouts (legitimate queries) | -80% | User reports |
| Runaway query protection | Maintained | p99 latency |
| Operator configuration burden | -70% | Survey |

## Effort Estimation

- **Total**: 15 dev-days
- **Statistics**: 5 days
- **Timeout logic**: 5 days
- **Observability**: 3 days
- **Testing**: 2 days

## Required Approvals

- [ ] SQL Execution Team
- [ ] Observability Team
- [ ] Product (for UX)

## Rollback Strategy

```sql
SET CLUSTER SETTING sql.adaptive_timeout.enabled = false;
```

Immediate fallback to static timeouts.

## References

- PostgreSQL statement_timeout
- MySQL adaptive timeout proposals
- Query performance prediction literature

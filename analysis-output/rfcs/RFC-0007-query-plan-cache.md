# RFC-0007: Query Plan Cache for Repeated Queries

**Status:** Draft
**Author:** Codebase Analysis
**Created:** November 19, 2025
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Summary

Implement a query plan cache that stores and reuses optimized query plans for repeated queries, reducing optimization overhead for common query patterns.

## Motivation

### Current State

CockroachDB optimizes every query execution:

```
SELECT * FROM users WHERE id = $1
  → Parse → Build Memo → Optimize → Execute
```

For a query executed 1000 times/second, we optimize 1000 times.

### Problem

- **Optimization overhead**: Complex queries take 10-100ms to optimize
- **CPU waste**: Same optimization repeated
- **Latency impact**: Especially for prepared statements

### Proposed Solution

Cache optimized plans by query fingerprint:

```
First execution:
  Parse → Build → Optimize → Cache → Execute

Subsequent executions:
  Parse → Cache lookup → Execute
```

## Detailed Design

### Plan Cache Structure

```go
type PlanCache struct {
    mu      sync.RWMutex
    entries map[string]*CacheEntry  // fingerprint → entry
    lru     *list.List
    maxSize int
}

type CacheEntry struct {
    Fingerprint  string
    Plan         memo.RelExpr
    Stats        PlanStats
    ValidSince   time.Time
    Dependencies []descpb.ID  // Tables used
}
```

### Cache Key

Use query fingerprint + relevant settings:

```go
type CacheKey struct {
    Fingerprint string
    Database    string
    SearchPath  string
    // Settings that affect optimization
    DistSQLMode string
    VectorizeMode string
}
```

### Cache Operations

#### Lookup

```go
func (c *PlanCache) Lookup(key CacheKey, catalog cat.Catalog) (*CacheEntry, bool) {
    c.mu.RLock()
    entry, ok := c.entries[key.Hash()]
    c.mu.RUnlock()

    if !ok {
        return nil, false
    }

    // Check if still valid
    if !c.isValid(entry, catalog) {
        c.Invalidate(key)
        return nil, false
    }

    return entry, true
}
```

#### Insertion

```go
func (c *PlanCache) Insert(key CacheKey, plan memo.RelExpr, deps []descpb.ID) {
    c.mu.Lock()
    defer c.mu.Unlock()

    // Evict if at capacity
    if len(c.entries) >= c.maxSize {
        c.evictLRU()
    }

    c.entries[key.Hash()] = &CacheEntry{
        Fingerprint:  key.Fingerprint,
        Plan:         plan,
        ValidSince:   time.Now(),
        Dependencies: deps,
    }
}
```

### Invalidation

Plans are invalidated when:
1. Schema changes (table/index modified)
2. Statistics updated
3. Settings changed

```go
func (c *PlanCache) InvalidateTable(tableID descpb.ID) {
    c.mu.Lock()
    defer c.mu.Unlock()

    for key, entry := range c.entries {
        for _, dep := range entry.Dependencies {
            if dep == tableID {
                delete(c.entries, key)
                break
            }
        }
    }
}
```

### Integration with Optimizer

```go
// pkg/sql/opt/optbuilder/builder.go
func (b *Builder) Build() error {
    // Check cache first
    if entry, ok := b.planCache.Lookup(b.cacheKey, b.catalog); ok {
        b.useCachedPlan(entry)
        return nil
    }

    // Normal optimization
    if err := b.buildInternal(); err != nil {
        return err
    }

    // Cache result
    b.planCache.Insert(b.cacheKey, b.memo.RootExpr(), b.dependencies)
    return nil
}
```

### Cache Scope

Two levels:
1. **Session cache**: Per-connection, no synchronization
2. **Cluster cache**: Shared across connections, requires synchronization

```go
type SessionPlanCache struct {
    entries map[string]*CacheEntry
    maxSize int  // Default: 100
}

type ClusterPlanCache struct {
    entries sync.Map
    size    atomic.Int64
    maxSize int  // Default: 10000
}
```

### Configuration

```sql
-- Enable plan cache
SET CLUSTER SETTING sql.plan_cache.enabled = true;

-- Configure sizes
SET CLUSTER SETTING sql.plan_cache.session_size = 100;
SET CLUSTER SETTING sql.plan_cache.cluster_size = 10000;

-- Session-level disable
SET plan_cache = off;
```

### Monitoring

```sql
-- View cache statistics
SELECT * FROM crdb_internal.plan_cache_stats;

-- View cached plans
SELECT
    fingerprint,
    plan_gist,
    hit_count,
    last_used
FROM crdb_internal.plan_cache;
```

### Metrics

```go
var (
    planCacheHits = metric.NewCounter(metadata)
    planCacheMisses = metric.NewCounter(metadata)
    planCacheEvictions = metric.NewCounter(metadata)
    planCacheSize = metric.NewGauge(metadata)
)
```

## Example Usage

### Automatic Caching

```go
// First execution: optimizes and caches
db.Query("SELECT * FROM users WHERE id = $1", 5)
// Cache miss, optimization: 10ms

// Second execution: uses cache
db.Query("SELECT * FROM users WHERE id = $1", 6)
// Cache hit, no optimization: 0.1ms
```

### Prepared Statements

```go
stmt, _ := db.Prepare("SELECT * FROM orders WHERE user_id = $1")
// Parse and optimize once

for i := 0; i < 1000; i++ {
    stmt.Query(i)  // Execute cached plan
}
```

### Cache Invalidation

```sql
-- Modify table
ALTER TABLE users ADD COLUMN email STRING;
-- Plans using users are invalidated

-- New execution re-optimizes
SELECT * FROM users WHERE id = $1;
-- Cache miss, re-optimize with new schema
```

## Implementation Plan

### Phase 1: Session Cache (1 week)
- [ ] Implement session-level cache
- [ ] Integrate with optimizer
- [ ] Basic invalidation

### Phase 2: Cluster Cache (1 week)
- [ ] Shared cache structure
- [ ] Synchronization
- [ ] Distributed invalidation

### Phase 3: Invalidation Logic (1 week)
- [ ] Schema change detection
- [ ] Statistics update detection
- [ ] Broadcast invalidation

### Phase 4: Observability (3 days)
- [ ] Metrics
- [ ] Virtual tables
- [ ] Logging

### Phase 5: Testing (4 days)
- [ ] Unit tests
- [ ] Integration tests
- [ ] Performance validation

## Backwards Compatibility

- Default disabled
- Opt-in via cluster setting
- No external API changes

## Alternatives Considered

### 1. Prepared Statement Only

**Pros**: Simpler
**Cons**: Requires application changes
**Decision**: Cache all for maximum benefit

### 2. Plan Hinting

**Pros**: User control
**Cons**: Manual management
**Decision**: Automatic caching better UX

### 3. Result Caching

**Pros**: Even faster
**Cons**: Invalidation complexity, memory
**Decision**: Plan cache is right granularity

## Open Questions

1. **Cache granularity**: Should settings affect cache key?
2. **Memory bound**: How to limit memory usage?
3. **Distributed invalidation**: How to sync across nodes?
4. **Plan staleness**: When is a plan too old?

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| Optimization time reduction | -40% | CPU profile |
| Cache hit rate | >80% | Cache metrics |
| Latency improvement | -10% | p50 latency |
| Memory overhead | <100MB | Memory metrics |

## Effort Estimation

- **Total**: 20 dev-days
- **Session cache**: 5 days
- **Cluster cache**: 5 days
- **Invalidation**: 5 days
- **Observability**: 2 days
- **Testing**: 3 days

## Required Approvals

- [ ] SQL Optimizer Team
- [ ] Performance Team

## Rollback Strategy

```sql
SET CLUSTER SETTING sql.plan_cache.enabled = false;
```

Immediate disable, all queries re-optimize.

## References

- PostgreSQL plan cache
- Oracle cursor sharing
- SQL Server plan cache
- CockroachDB prepared statement handling

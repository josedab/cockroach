# Understanding CockroachDB: Architecture and Core Concepts

**Series:** Understanding CockroachDB Internals (Part 1 of 6)
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## What You'll Learn

- CockroachDB's layered architecture and why it was chosen
- Key abstractions that make distributed SQL possible
- Trade-offs between consistency, availability, and performance
- How to navigate the codebase effectively

---

## Introduction

CockroachDB tackles one of the hardest problems in distributed systems: providing the familiar interface of a SQL database while transparently handling the complexities of distribution, replication, and fault tolerance. Understanding its architecture is essential for anyone working with or contributing to the project.

In this post, we'll explore how CockroachDB organizes its ~8,600 Go source files into a coherent system that serves SQL queries across a distributed cluster.

## The Problem Domain

Traditional SQL databases are single-node systems. Scaling them requires manual sharding, which pushes complexity to application developers. NoSQL databases scale horizontally but sacrifice SQL's powerful query capabilities and transactional guarantees.

CockroachDB aims to provide:
- **PostgreSQL compatibility**: Familiar SQL interface
- **Horizontal scalability**: Add nodes to increase capacity
- **Strong consistency**: Serializable isolation by default
- **Fault tolerance**: Survive node, rack, or datacenter failures

These goals create tension. Strong consistency typically requires coordination, which hurts performance. CockroachDB's architecture represents carefully considered trade-offs to balance these competing concerns.

## Layered Architecture

CockroachDB uses a layered architecture with clear separation of concerns:

```
┌─────────────────────────────────────────────┐
│              SQL Layer                       │
│  Parse → Optimize → Execute                  │
│  pkg/sql/                                    │
├─────────────────────────────────────────────┤
│          Transaction Layer                   │
│  Coordinate → Route → Retry                  │
│  pkg/kv/kvclient/                           │
├─────────────────────────────────────────────┤
│          Replication Layer                   │
│  Raft Consensus → Replica Management         │
│  pkg/kv/kvserver/                           │
├─────────────────────────────────────────────┤
│           Storage Layer                      │
│  MVCC → Pebble (LSM-tree)                   │
│  pkg/storage/                               │
└─────────────────────────────────────────────┘
```

### Why Layering?

**Separation of concerns**: Each layer handles specific complexity. The SQL layer doesn't need to know about Raft; the storage layer doesn't need to know about query optimization.

**Testability**: Layers can be tested independently. The optimizer doesn't need a real cluster to verify plan quality.

**Evolution**: Layers can change implementation without affecting others. The storage engine could be swapped (RocksDB → Pebble) without rewriting SQL.

**Trade-off**: Layering adds indirection. A query crosses multiple abstraction boundaries, which can impact performance. CockroachDB mitigates this with careful API design and optimization across layers.

## The SQL Layer

The SQL layer transforms PostgreSQL-compatible queries into key-value operations. It's the most code-heavy layer, with approximately 2,000 files in `pkg/sql/`.

### Request Flow

```
Client SQL → Parser → AST → Optimizer → Memo →
Execution Builder → Plan → Executor → Results
```

**Entry Point**: [`pkg/sql/conn_executor.go:1529`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/conn_executor.go#L1529)

The `connExecutor` is the main coordinator. For each statement, it:

1. Parses SQL text into an AST
2. Builds a memo (forest of equivalent expressions)
3. Optimizes to find the lowest-cost plan
4. Builds an execution plan
5. Executes and streams results

### The Optimizer

CockroachDB uses a Cascades-style optimizer with a crucial innovation: **memo-based expression interning**.

```go
// pkg/sql/opt/memo/memo.go:116-150
type Memo struct {
    // Forest of logically equivalent expressions
    groups []group
    // Interning ensures each unique expression exists once
    interner interner
}
```

**Why this matters**: A complex query might have millions of logically equivalent plans. Naive representation would explode memory. The memo uses hash-based interning to ensure each unique expression exists exactly once, enabling O(1) equivalence checks via pointer comparison.

See [`pkg/sql/opt/memo/memo.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/opt/memo/memo.go#L116).

## The Transaction Layer

The transaction layer (`pkg/kv/`) provides distributed transactions with Serializable Snapshot Isolation (SSI). This is where CockroachDB's distributed nature becomes apparent.

### Key Abstraction: TxnCoordSender

The `TxnCoordSender` coordinates a transaction's lifecycle using an **interceptor stack**:

```go
// pkg/kv/kvclient/kvcoord/txn_coord_sender.go:113-179
type TxnCoordSender struct {
    // Interceptor stack for pluggable transaction behaviors
    interceptorStack [7]txnInterceptor
    // Transaction state
    mu struct {
        txn roachpb.Transaction
        // ...
    }
}
```

**Interceptors** (in order):
1. **Heartbeater**: Keeps transaction alive
2. **Seq Num Allocator**: Assigns sequence numbers to operations
3. **Pipeliner**: Enables write pipelining
4. **Span Refresher**: Handles read-refresh on conflicts
5. **Committer**: Manages commit protocol
6. **Metrics**: Collects transaction metrics
7. **Lock Footprint**: Tracks lock spans

**Why an interceptor stack?** Each concern (heartbeating, pipelining, refresh) is complex enough to warrant isolation. The stack pattern enables:
- Independent testing of each interceptor
- Pluggable behaviors for different transaction types
- Clear responsibility boundaries

**Trade-off**: Indirection adds overhead. Each request passes through all interceptors. This is acceptable because the cost is small compared to network round-trips.

See [`pkg/kv/kvclient/kvcoord/txn_coord_sender.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/kvclient/kvcoord/txn_coord_sender.go#L113).

## The Replication Layer

CockroachDB partitions data into **ranges** (typically ~512MB). Each range is replicated across multiple nodes using Raft consensus.

### Range-Based Sharding

```
Key Space: [MinKey ─────────────────────── MaxKey]
                    │           │           │
              Range 1     Range 2     Range 3
              [a-m)        [m-t)       [t-∞)
```

**Why ranges?** Compared to hash-based sharding:

| Aspect | Range-Based | Hash-Based |
|--------|-------------|------------|
| Range scans | Efficient (sequential) | Expensive (scattered) |
| Hotspots | Possible | Distributed |
| Rebalancing | Split/merge | Rehash all |

CockroachDB chose ranges because SQL workloads frequently need range scans. Hotspots are mitigated with automatic splitting and load-based rebalancing.

### Raft per Range

Each range runs its own Raft group for consensus:

```go
// pkg/kv/kvserver/replica.go:353
type Replica struct {
    RangeID roachpb.RangeID
    mu      struct {
        state storagepb.ReplicaState
        // Raft state machine
        // ...
    }
}
```

**Trade-off**: Running Raft per range adds overhead (one Raft group per ~512MB of data). However, this enables:
- Independent failure handling per range
- Parallelism across ranges
- Fine-grained lease management

A cluster with 10TB of data has ~20,000 ranges. Managing 20,000 Raft groups is challenging, which led to optimizations like Raft log batching and lazy leader election.

See [`pkg/kv/kvserver/replica.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/kvserver/replica.go#L353).

## The Storage Layer

The storage layer (`pkg/storage/`) provides MVCC (Multi-Version Concurrency Control) on top of Pebble, an LSM-tree storage engine.

### MVCC Keys

```go
// pkg/storage/mvcc_key.go:29-143
type MVCCKey struct {
    Key       roachpb.Key
    Timestamp hlc.Timestamp
}
```

Keys are ordered by `(key, -timestamp)`, so the newest version comes first. This enables efficient point-in-time reads.

**Why MVCC?** It enables:
- Non-blocking reads (readers don't block writers)
- Historical queries (AS OF SYSTEM TIME)
- Efficient garbage collection

See [`pkg/storage/mvcc_key.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/storage/mvcc_key.go#L29).

## Cross-Cutting Concerns

Several concerns span all layers:

### Observability

CockroachDB has comprehensive observability built into every layer:

- **Logging**: Channel-based routing (DEV, OPS, SQL_PERF, etc.)
- **Metrics**: Prometheus-compatible export
- **Tracing**: OpenTelemetry integration

```go
// pkg/util/log/ambient_context.go:50-115
type AmbientContext struct {
    Tracer    *tracing.Tracer
    ServerIDs serverident.ServerIdentificationPayload
    tags      *logtags.Buffer
}
```

The `AmbientContext` pattern propagates logging/tracing context through the entire request path.

### Error Handling

Errors are hierarchical and informative:

```go
// Errors from pkg/kv/kvpb/errors.proto
- TransactionRetryError
  - WriteTooOldError
  - ReadWithinUncertaintyIntervalError
- ConditionFailedError
- IntentMissingError
```

Each error type carries context that helps the transaction coordinator decide how to handle it (retry, abort, refresh timestamp).

## Architectural Trade-offs

| Decision | Trade-off | Why CockroachDB Chose This |
|----------|-----------|---------------------------|
| Serializable Isolation | Performance vs Consistency | Correctness is paramount for target use cases (financial, etc.) |
| Range-based Sharding | Hotspots vs Scan Efficiency | SQL workloads need efficient range scans |
| Raft per Range | Overhead vs Granularity | Enables independent failure handling and parallelism |
| Interceptor Stack | Indirection vs Modularity | Complexity of each concern warrants isolation |
| MVCC | Storage overhead vs Concurrency | Non-blocking reads are essential for performance |

## Navigating the Codebase

### Entry Points

- **Server startup**: [`pkg/cli/start.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/cli/start.go)
- **SQL execution**: [`pkg/sql/conn_executor.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/conn_executor.go)
- **Transaction handling**: [`pkg/kv/txn.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/txn.go)
- **Replica management**: [`pkg/kv/kvserver/replica.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/kvserver/replica.go)

### Package Conventions

- `doc.go`: Package documentation
- `*_test.go`: Tests colocated with source
- `*.proto`: Protocol buffer definitions
- `*_generated.go`: Generated code

## Key Takeaways

1. **Layered architecture enables evolution**: Each layer can change independently (e.g., RocksDB → Pebble).

2. **Range-based sharding optimizes for SQL**: Range scans are common in SQL, so contiguous storage is worth the hotspot risk.

3. **Interceptors manage transaction complexity**: Seven interceptors handle concerns from heartbeating to commit protocols.

4. **MVCC enables non-blocking reads**: Historical versions support point-in-time queries and concurrent access.

5. **Trade-offs are explicit**: CockroachDB documents why it chose consistency over availability, ranges over hashing, etc.

## What's Next

In the next post, we'll dive deep into the SQL layer—parsing, optimization, and execution. We'll see how a `SELECT` statement transforms from text into a distributed execution plan.

---

## Further Reading

- [CockroachDB Architecture Overview](https://www.cockroachlabs.com/docs/stable/architecture/overview.html)
- [Design Documents](/docs/design.md)
- [Technical Notes](/docs/tech-notes/)

---

*This is Part 1 of the "Understanding CockroachDB Internals" series. Continue to Part 2: SQL Layer Deep Dive.*

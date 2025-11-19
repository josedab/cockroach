# CockroachDB Terminology Glossary

**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Core Concepts

### Range
A contiguous chunk of key-value data, typically ~512MB. Each range:
- Has a unique RangeID
- Maintains multiple replicas across nodes
- Uses Raft for consensus
- Can split/merge automatically

### Leaseholder
The replica in a range's Raft group that holds the "lease" and serves reads/writes. The leaseholder:
- Coordinates writes
- Serves consistent reads without Raft round-trips
- Transfers automatically on failure

### Intent
A provisional write created during a transaction. Intents:
- Contain the transaction's metadata
- Are resolved (committed or aborted) after the transaction completes
- Block conflicting readers/writers

### MVCC (Multi-Version Concurrency Control)
Storage mechanism where each value has a timestamp. Enables:
- Non-blocking reads at historical timestamps
- Garbage collection of old versions
- Point-in-time recovery

### SSI (Serializable Snapshot Isolation)
CockroachDB's default isolation level. Provides:
- Serializable semantics (strongest isolation)
- Snapshot reads within a transaction
- Conflict detection for write-write and write-read conflicts

## SQL Layer Terms

### AST (Abstract Syntax Tree)
Parsed representation of a SQL statement before optimization. Defined in `pkg/sql/sem/tree/`.

### Memo
Data structure in the optimizer that stores logically equivalent query expressions efficiently. Uses expression interning to avoid duplication.

### OptExpr
Optimized expression tree output by the query optimizer. Input to the execution builder.

### Executor
Component that runs optimized query plans. Two types:
- **Row-based**: Traditional tuple-at-a-time execution
- **Columnar (Vectorized)**: Batch-oriented, SIMD-friendly execution

### connExecutor
The main state machine coordinating SQL execution for a client connection. Manages:
- Statement parsing and execution
- Transaction state
- Automatic retry
- Result delivery

### DistSQL
Distributed SQL execution framework. Spreads query processing across multiple nodes for parallel execution.

## KV Layer Terms

### TxnCoordSender
Transaction coordinator that manages a transaction's lifecycle. Uses an interceptor stack for:
- Heartbeating
- Sequence allocation
- Write buffering
- Conflict resolution

### DistSender
Routes KV requests to the appropriate range's leaseholder. Caches range descriptors for efficiency.

### Interceptor
Pluggable component in the transaction processing pipeline. Each interceptor handles specific concerns (heartbeat, pipelining, refresh).

### BatchRequest
A collection of KV operations sent together. Enables:
- Reduced network round-trips
- Atomic execution across multiple keys

### RootTxn / LeafTxn
- **RootTxn**: Main transaction coordinator (performs heartbeating)
- **LeafTxn**: Distributed execution transaction (no heartbeating, reports back to root)

## Storage Layer Terms

### Pebble
CockroachDB's storage engine (LSM-tree based). Fork of RocksDB rewritten in Go.

### MVCCKey
Key format: `user_key + timestamp`. Ordered by (key, -timestamp) so newest versions come first.

### Write Intent
An uncommitted write stored as an MVCC value with a special marker linking to the transaction record.

### Lock Table
In-memory data structure tracking active locks on keys. Used for:
- Detecting conflicts
- Queuing waiting requests
- Deadlock detection

### Latch
Short-term lock on key spans during command evaluation. Prevents concurrent modifications to the same keys.

## Replication Terms

### Raft
Consensus protocol for replicating range data. Each range has its own Raft group.

### Raft Log
Ordered sequence of commands that all replicas apply to reach consensus.

### Snapshot
Full copy of a range's data, used to bring a new replica up to date.

### Quorum
Majority of replicas needed to commit a Raft entry. For 3 replicas, quorum = 2.

## Schema Change Terms

### Element
Atomic piece of schema in the declarative schema changer (Column, Index, Constraint, etc.).

### Element Status
State of a schema element: `PUBLIC`, `ABSENT`, `DELETE_ONLY`, `WRITE_ONLY`, `BACKFILLING`, etc.

### 2-Version Invariant
Constraint that at most two schema versions exist simultaneously. Ensures:
- New writers don't create data old readers can't see
- Old readers don't miss data new writers create

### Schema Change Job
Asynchronous job that executes schema changes across the cluster.

## Observability Terms

### Log Channel
Destination for log messages (DEV, OPS, HEALTH, SQL_PERF, etc.). Each targets different audiences.

### Metric
Quantitative measurement exported to Prometheus:
- **Counter**: Monotonically increasing
- **Gauge**: Point-in-time value
- **Histogram**: Distribution over time

### Span
Unit of work in distributed tracing. Contains:
- Operation name
- Start/end time
- Tags and logs
- Parent/child relationships

### AmbientContext
Pattern for propagating logging/tracing context through the codebase. Embeds tracer and log tags.

## Testing Terms

### Logic Test
SQL correctness test using a simple DSL:
```
statement ok
CREATE TABLE t (a INT)

query I
SELECT * FROM t
----
```

### Roachtest
Integration test that spins up real clusters. Tests distributed scenarios like failover, upgrades.

### TestCluster
Multi-node test cluster for integration testing. Provides access to SQL connections and internal interfaces.

## Build System Terms

### Bazel
Build tool used by CockroachDB. Provides:
- Hermetic builds
- Distributed caching
- Cross-compilation

### Gazelle
Tool that auto-generates BUILD.bazel files from Go source.

### Dev Tool
Wrapper script (`./dev`) that invokes Bazel with correct flags.

## Deployment Terms

### Node
A single CockroachDB process/server. A cluster has multiple nodes.

### Store
Storage directory on a node. A node can have multiple stores (one per disk).

### Locality
Hierarchical location (region/zone/rack) used for:
- Replica placement
- Query routing
- Survival goals

### Virtual Cluster (Tenant)
Isolated database environment sharing physical infrastructure. Used for multi-tenancy.

## Transaction Terms

### Read Timestamp
Timestamp at which a transaction reads data. Determines snapshot visibility.

### Write Timestamp
Timestamp at which a transaction's writes are committed.

### Transaction Record
Persistent record tracking a transaction's state (PENDING, COMMITTED, ABORTED).

### Parallel Commit
Optimization that commits a transaction without waiting for intent resolution.

### Refresh
Optimistic retry of a transaction at a higher timestamp when pushed by a conflict.

## Protocol Terms

### pgwire
PostgreSQL wire protocol implementation. Enables compatibility with PostgreSQL clients.

### gRPC
Protocol for inter-node communication. Used for KV requests, Raft messages, etc.

### Gossip
Protocol for cluster metadata propagation. Nodes share information about:
- Node liveness
- Store capacity
- System configuration

---

## Acronyms

| Acronym | Expansion |
|---------|-----------|
| AST | Abstract Syntax Tree |
| CCL | Cockroach Core License (enterprise features) |
| CDC | Change Data Capture |
| DDL | Data Definition Language |
| DML | Data Manipulation Language |
| KV | Key-Value |
| LSM | Log-Structured Merge-tree |
| MVCC | Multi-Version Concurrency Control |
| OTEL | OpenTelemetry |
| RPC | Remote Procedure Call |
| SSI | Serializable Snapshot Isolation |
| SST | Sorted String Table |
| TTL | Time To Live |
| UDF | User-Defined Function |

---

*This glossary covers the most commonly used terms in the CockroachDB codebase. For protocol-specific terminology, see the respective `.proto` files.*

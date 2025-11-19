# CockroachDB Codebase Quick Start Guide

**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`
**Analysis Date:** November 19, 2025

---

## What is CockroachDB?

CockroachDB is a distributed SQL database written in Go that provides:
- PostgreSQL wire protocol compatibility
- Serializable Snapshot Isolation (strongest isolation level)
- Horizontal scalability through automatic sharding
- Fault tolerance via Raft consensus

## Architecture at a Glance

```
┌─────────────────────────────────────────────────┐
│                  SQL Layer                       │
│  Parser → Optimizer → Executor                   │
│  pkg/sql/                                        │
├─────────────────────────────────────────────────┤
│              Distribution Layer                  │
│  Transaction Coordination, Range Routing         │
│  pkg/kv/kvclient/                               │
├─────────────────────────────────────────────────┤
│               Replication Layer                  │
│  Raft Consensus, Replica Management              │
│  pkg/kv/kvserver/                               │
├─────────────────────────────────────────────────┤
│               Storage Layer                      │
│  MVCC, Pebble (LSM-tree)                        │
│  pkg/storage/                                    │
└─────────────────────────────────────────────────┘
```

## Key Numbers

| Metric | Count |
|--------|-------|
| Go source files | 8,600 |
| Test files | 2,873 |
| Protocol buffer files | 171 |
| Repository size | 752 MB |

## Essential Commands

```bash
# Verify development environment
./dev doctor

# Build (fast, no UI)
./dev build short

# Build full binary
./dev build cockroach

# Run tests
./dev test pkg/sql -f=TestParse -v

# Run SQL logic tests
./dev testlogic base --files='select'

# Generate code after proto/grammar changes
./dev generate go

# Lint (takes a while)
./dev lint --short
```

## Core Concepts

### 1. Ranges
Data is partitioned into ~512MB chunks called "ranges." Each range:
- Has its own Raft group for consensus
- Can be split/merged automatically
- Replicated across nodes (default: 3 replicas)

### 2. Transactions
All operations use distributed transactions with:
- Serializable isolation (default)
- Automatic retry on conflicts
- Two-phase commit with parallel commits optimization

### 3. MVCC
Multi-Version Concurrency Control enables:
- Non-blocking reads
- Point-in-time queries
- Efficient garbage collection

## Entry Points

### Server Startup
- [`pkg/cli/start.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/cli/start.go) - CLI entry
- [`pkg/server/server.go:272`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/server/server.go#L272) - Server initialization

### SQL Processing
- [`pkg/sql/parser/parse.go:88`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/parser/parse.go#L88) - SQL parsing
- [`pkg/sql/opt/xform/optimizer.go:250`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/opt/xform/optimizer.go#L250) - Query optimization
- [`pkg/sql/conn_executor.go:1529`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/conn_executor.go#L1529) - Statement execution

### KV Operations
- [`pkg/kv/txn.go:71`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/txn.go#L71) - Transaction interface
- [`pkg/kv/kvclient/kvcoord/txn_coord_sender.go:113`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/kvclient/kvcoord/txn_coord_sender.go#L113) - Transaction coordinator
- [`pkg/kv/kvserver/replica.go:353`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/kvserver/replica.go#L353) - Replica management

## Key Design Patterns

1. **Interceptor Stack** - Transaction processing uses pluggable interceptors
2. **Factory Pattern** - Object creation throughout (TxnCoordSenderFactory, etc.)
3. **Command Pattern** - KV operations registered as commands
4. **Memo Pattern** - Query optimizer uses memo for expression interning

## Testing Strategy

| Type | Location | Purpose |
|------|----------|---------|
| Unit | `*_test.go` | Function-level testing |
| Logic | `pkg/sql/logictest/` | SQL correctness |
| Roachtest | `pkg/cmd/roachtest/` | Distributed integration |
| Benchmark | `pkg/bench/` | Performance |

## Configuration

- **Cluster Settings**: Runtime-adjustable via `SET CLUSTER SETTING`
- **Command Flags**: `--store`, `--listen-addr`, `--join`, etc.
- **Environment Variables**: `COCKROACH_*` prefix

## What's Next?

1. **Read the Blog Series** - Start with `blog-series/01-architecture-overview.md`
2. **Explore Specific Areas** - Use the repository structure guide
3. **Review RFCs** - Check improvement proposals in `rfcs/`

## Getting Help

- **Documentation**: https://cockroachlabs.com/docs/stable/
- **Architecture Guide**: https://www.cockroachlabs.com/docs/stable/architecture/overview.html
- **Contributing**: See `/CONTRIBUTING.md`

---

*This quick start guide provides an overview. For detailed analysis, see the full blog series and RFC documents.*

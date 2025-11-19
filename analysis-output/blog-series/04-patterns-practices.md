# Patterns and Practices in CockroachDB

**Series:** Understanding CockroachDB Internals (Part 4 of 6)
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## What You'll Learn

- Design patterns used throughout the codebase
- Testing strategies for distributed systems
- Error handling and resilience patterns
- Code organization principles for navigating the codebase

---

## Introduction

Building a distributed database requires managing enormous complexity. CockroachDB uses well-established design patterns to keep this complexity under control. In this post, we'll explore the patterns that make the codebase navigable and maintainable.

## Design Patterns

### 1. Interceptor Pattern

The most distinctive pattern in CockroachDB is the **interceptor stack** used for transaction processing.

**Location**: [`pkg/kv/kvclient/kvcoord/txn_coord_sender.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/kvclient/kvcoord/txn_coord_sender.go#L113)

```go
type txnInterceptor interface {
    lockedSender
    // Called when transaction is created
    epochBumpedLocked()
    // Called when transaction is closed
    closeLocked()
}
```

Each interceptor:
1. Receives a request
2. Optionally modifies it
3. Passes to the next interceptor
4. Optionally modifies the response

**Benefits**:
- Single responsibility per interceptor
- Independent testing
- Pluggable behaviors

**When to use**: When you have a pipeline of concerns that each request must pass through.

### 2. Factory Pattern

Factories appear throughout for creating complex objects:

```go
// pkg/kv/kvclient/kvcoord/txn_coord_sender_factory.go
type TxnCoordSenderFactory struct {
    cfg TxnCoordSenderFactoryConfig
}

func (f *TxnCoordSenderFactory) RootTransactionalSender(
    txn *roachpb.Transaction, pri roachpb.UserPriority,
) kv.TxnSender {
    // Create and wire up TxnCoordSender with all interceptors
}
```

**Benefits**:
- Encapsulates complex creation logic
- Ensures consistent configuration
- Enables dependency injection for testing

### 3. Command Pattern

KV operations are registered as commands:

**Location**: [`pkg/kv/kvserver/batcheval/command.go:50-123`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/kvserver/batcheval/command.go#L50)

```go
type Command struct {
    // Key spans this command accesses
    DeclareKeys func(roachpb.RangeDescriptor, *roachpb.Header,
                     roachpb.Request, *SpanSet, *SpanSet, int64)

    // Evaluate for read-write commands
    EvalRW func(context.Context, ReadWriter, CommandArgs,
                roachpb.Response) (result.Result, error)

    // Evaluate for read-only commands
    EvalRO func(context.Context, Reader, CommandArgs,
                roachpb.Response) (result.Result, error)
}

// 60+ registered commands
var commands = map[roachpb.Method]Command{
    roachpb.Get: {...},
    roachpb.Put: {...},
    roachpb.Scan: {...},
    // ...
}
```

**Benefits**:
- Uniform handling for all operations
- Easy to add new commands
- Clear separation of declaration and execution

### 4. Registry Pattern

Used for metrics, settings, and commands:

```go
// pkg/util/metric/registry.go:40-120
type Registry struct {
    mu      syncutil.Mutex
    tracked map[string]Iterable
    labels  []labelPair
}

func (r *Registry) AddMetric(metric Iterable) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.tracked[metric.GetName()] = metric
}
```

**Use cases**:
- Cluster settings (`pkg/settings/`)
- Metrics (`pkg/util/metric/`)
- Commands (`pkg/kv/kvserver/batcheval/`)

### 5. Strategy Pattern

Swappable implementations behind interfaces:

```go
// pkg/kv/sender.go:53
type Sender interface {
    Send(context.Context, *roachpb.BatchRequest) (*roachpb.BatchResponse, *roachpb.Error)
}

// Multiple implementations:
// - TxnCoordSender (transaction coordination)
// - DistSender (range routing)
// - replica (local execution)
```

**Benefits**:
- Implementations can be swapped for testing
- Clear contracts between components
- Enables composition

### 6. Monitor Pattern

For resource tracking:

```go
// pkg/util/mon/bytes_monitor.go
type BytesMonitor struct {
    name     string
    limit    int64
    reserved int64
    mu       struct {
        syncutil.Mutex
        curAllocated int64
    }
}

func (mm *BytesMonitor) AllocBytes(ctx context.Context, x int64) error {
    // Track allocation, return error if over limit
}
```

Used for:
- Per-session memory
- Per-transaction memory
- Per-result memory

## Testing Strategies

CockroachDB has a comprehensive testing infrastructure.

### Unit Tests

Standard Go tests, colocated with source:

```go
// pkg/kv/txn_test.go
func TestTxnCommit(t *testing.T) {
    defer leaktest.AfterTest(t)()
    // ...
}
```

**Pattern**: Always use `leaktest.AfterTest(t)()` to detect goroutine leaks.

### Test Clusters

**Location**: [`pkg/testutils/testcluster/testcluster.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/testutils/testcluster/testcluster.go)

```go
func TestDistributedQuery(t *testing.T) {
    tc := testcluster.StartTestCluster(t, 3, base.TestClusterArgs{})
    defer tc.Stopper().Stop(context.Background())

    db := tc.ServerConn(0)
    // Test against 3-node cluster
}
```

**Use cases**: Testing distributed scenarios, replication, failover.

### Logic Tests

SQL correctness tests using a declarative DSL:

```
statement ok
CREATE TABLE t (a INT PRIMARY KEY)

statement ok
INSERT INTO t VALUES (1), (2), (3)

query I rowsort
SELECT * FROM t
----
1
2
3
```

**Location**: `pkg/sql/logictest/testdata/logic_test/`

**Benefits**:
- Easy to write and read
- Runs under multiple configurations (single-node, distributed, vectorized)
- Catches regressions

### Roachtests

Integration tests for distributed scenarios:

```go
// pkg/cmd/roachtest/tests/
func registerSchemaChange(r registry.Registry) {
    r.Add(registry.TestSpec{
        Name: "schemachange/concurrent",
        Run: func(ctx context.Context, t test.Test, c cluster.Cluster) {
            // Spin up real cluster
            // Run concurrent schema changes
            // Verify correctness
        },
    })
}
```

**Use cases**: Failover, upgrades, load testing, chaos testing.

### Metamorphic Testing

Randomized testing configurations:

```go
// pkg/util/metamorphic/metamorphic.go
var reusePercent = metamorphic.ConstantWithTestRange(
    "span-reuse-rate", 100, 0, 101,
)
```

Varies parameters across test runs to find edge cases.

## Error Handling

### Error Types

Errors are hierarchical and informative:

```go
// pkg/kv/kvpb/errors.proto
message TransactionRetryError {
    TransactionRetryReason reason = 1;
    string extra_msg = 2;
}

message WriteTooOldError {
    Timestamp actual_timestamp = 1;
}

message ConditionFailedError {
    Value actual_value = 1;
}
```

**Benefits**:
- Caller can make decisions based on error type
- Rich context for debugging
- Retryable vs fatal distinction

### Error Wrapping

Uses `cockroachdb/errors` for enhanced errors:

```go
import "github.com/cockroachdb/errors"

if err != nil {
    return errors.Wrap(err, "failed to read from store")
}

// Later
if errors.Is(err, context.Canceled) {
    // Handle cancellation
}
```

### Retry Logic

Standardized retry patterns:

```go
// pkg/util/retry/retry.go
for r := retry.StartWithCtx(ctx, opts); r.Next(); {
    if err := operation(); err == nil {
        break
    }
    // Exponential backoff with jitter
}
```

## Code Organization

### Package Structure

```
pkg/
├── sql/           # SQL layer (user-facing)
├── kv/            # Key-value layer (distribution)
├── storage/       # Storage engine
├── server/        # Server management
├── util/          # Shared utilities
├── roachpb/       # Protocol buffers
└── testutils/     # Test infrastructure
```

**Principle**: Dependencies flow downward. `sql` depends on `kv`, not vice versa.

### File Naming

| Pattern | Purpose |
|---------|---------|
| `doc.go` | Package documentation |
| `*_test.go` | Tests |
| `*_generated.go` | Generated code |
| `*.proto` | Protocol buffers |

### Interface Location

Interfaces live with their consumers, not implementers:

```go
// pkg/kv/sender.go defines Sender interface
// Implementations in pkg/kv/kvclient/, pkg/kv/kvserver/
```

This follows the Go idiom of "accept interfaces, return structs."

## Concurrency Patterns

### Stopper Pattern

Coordinated shutdown:

```go
// pkg/util/stop/stopper.go
type Stopper struct {
    // Tracks goroutines
}

func (s *Stopper) RunWorker(ctx context.Context, f func(context.Context)) {
    s.mu.Lock()
    s.numWorkers++
    s.mu.Unlock()

    go func() {
        defer s.mu.Lock(); s.numWorkers--; s.mu.Unlock()
        f(ctx)
    }()
}

func (s *Stopper) Stop(ctx context.Context) {
    // Wait for all workers to finish
}
```

**Benefit**: Clean shutdown without goroutine leaks.

### Lock Ordering

CockroachDB enforces strict lock ordering to prevent deadlocks:

```go
// Always acquire locks in order: Store → Replica → Range
func (r *Replica) processCommand() {
    r.mu.Lock()
    defer r.mu.Unlock()
    // Never acquire Store lock here
}
```

**Tool**: `syncutil.DeadlockEnabled` enables deadlock detection in tests.

## Configuration

### Cluster Settings

Runtime-adjustable settings:

```go
// pkg/settings/bool.go
var TraceRedactable = settings.RegisterBoolSetting(
    settings.SystemVisible,
    "trace.redactable.enabled",
    "set to true to enable finer-grained redactability",
    false,
)

// Usage
if TraceRedactable.Get(&sv) {
    // Redact sensitive data
}
```

**Benefit**: Change behavior without restart.

### Environment Variables

For development/debugging:

```go
// pkg/util/envutil/env.go
var debugEnabled = envutil.EnvOrDefaultBool(
    "COCKROACH_DEBUG",
    false,
)
```

## Documentation Patterns

### Package Documentation

Every package has `doc.go`:

```go
// Package kv provides a key-value API for CockroachDB.
//
// The API provides a distributed, transactional key-value store
// with support for MVCC (multi-version concurrency control).
package kv
```

### Code Comments

Comments explain "why," not "what":

```go
// We refresh read spans here to avoid restarting the transaction
// when it was pushed by a concurrent writer. This optimization
// saves network round-trips in the common case where the refresh
// succeeds (no actual conflict).
if err := tc.refreshSpans(ctx); err != nil {
    return err
}
```

### Generated Code Markers

```go
// Code generated by optgen; DO NOT EDIT.
```

## Key Takeaways

1. **Interceptor pattern manages transaction complexity**: Seven focused interceptors are easier to understand than one monolithic coordinator.

2. **Testing at multiple levels**: Unit tests, logic tests, and roachtests each serve different purposes.

3. **Rich errors enable smart handling**: Error types carry enough information for retry decisions.

4. **Stopper prevents goroutine leaks**: All background goroutines use the stopper.

5. **Settings enable runtime tuning**: Cluster settings let operators adjust behavior without restarts.

## Navigating the Codebase

### Finding an Entry Point

1. Start with `pkg/cli/` for user commands
2. Follow to `pkg/server/` for initialization
3. Trace to `pkg/sql/` for query processing

### Understanding a Feature

1. Find the `doc.go` for overview
2. Look for interfaces to understand contracts
3. Find tests for usage examples

### Making Changes

1. Run `./dev doctor` to verify environment
2. Make changes
3. Run `./dev test pkg/affected -v`
4. Run `./dev generate` if needed
5. Run `./dev lint --short` before committing

## What's Next

In Part 5, we'll dive into CockroachDB's observability infrastructure—logging, metrics, and tracing that make operating a distributed database possible.

---

## Further Reading

- [Contributing Guide](/CONTRIBUTING.md)
- [Code Review Guidelines](https://wiki.crdb.io/wiki/spaces/CRDB/pages/code+review)
- [Testing Philosophy](https://wiki.crdb.io/wiki/spaces/CRDB/pages/testing)

---

*This is Part 4 of the "Understanding CockroachDB Internals" series. Continue to Part 5: Observability Deep Dive.*

# Distributed Transactions in CockroachDB

**Series:** Understanding CockroachDB Internals (Part 3 of 6)
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## What You'll Learn

- How CockroachDB achieves serializable isolation across a distributed cluster
- The TxnCoordSender interceptor stack architecture
- Write pipelining and parallel commits optimization
- Conflict resolution and automatic retry mechanisms

---

## Introduction

Distributed transactions are arguably the hardest problem CockroachDB solves. Traditional databases use locks and single-node coordination. Distributed databases often compromise on isolation (eventual consistency, read-your-writes). CockroachDB provides serializable isolation—the strongest guarantee—across a distributed system.

In this post, we'll explore how the KV layer (`pkg/kv/`) implements distributed transactions.

## Transaction Model

CockroachDB uses **Serializable Snapshot Isolation (SSI)**, which provides:
- **Snapshot reads**: Transactions see a consistent snapshot
- **Serializable writes**: All transactions appear to execute in some serial order
- **Conflict detection**: Write-write and read-write conflicts are detected

### Transaction Types

```go
// pkg/kv/txn.go:71-179
type Txn struct {
    db      *DB
    typ     TxnType  // RootTxn or LeafTxn
    sender  TxnSender
    // ...
}
```

**RootTxn**: Main coordinator. Handles heartbeating, commit decisions.

**LeafTxn**: Distributed execution helper. Reports back to root, no heartbeating.

## The TxnCoordSender

The `TxnCoordSender` is the heart of transaction coordination. It uses an **interceptor stack** to manage transaction lifecycle.

**Location**: [`pkg/kv/kvclient/kvcoord/txn_coord_sender.go:113-179`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/kv/kvclient/kvcoord/txn_coord_sender.go#L113)

```go
type TxnCoordSender struct {
    wrapped lockedSender

    // Interceptor stack
    interceptorStack []txnInterceptor

    mu struct {
        syncutil.Mutex
        txn            roachpb.Transaction
        txnState       txnState
        finalObservedStatus roachpb.TransactionStatus
    }
}
```

### Interceptor Stack

Seven interceptors process each request:

```
Request → Heartbeater → SeqNumAllocator → Pipeliner →
          SpanRefresher → Committer → Metrics → LockFootprint → DistSender
```

Let's examine each:

#### 1. Heartbeater
**File**: `pkg/kv/kvclient/kvcoord/txn_interceptor_heartbeater.go`

Keeps the transaction alive by periodically updating its record. Without heartbeats, the transaction would be aborted after a timeout.

```go
// Heartbeat every transaction liveness threshold / 2
func (h *txnHeartbeater) heartbeatLoop(ctx context.Context) {
    ticker := time.NewTicker(h.loopInterval)
    for {
        select {
        case <-ticker.C:
            h.heartbeat(ctx)
        case <-ctx.Done():
            return
        }
    }
}
```

#### 2. Sequence Number Allocator
**File**: `pkg/kv/kvclient/kvcoord/txn_interceptor_seq_num_allocator.go`

Assigns monotonically increasing sequence numbers to each operation. Enables:
- Ordering within a transaction
- Savepoint implementation
- Write intent resolution

#### 3. Pipeliner
**File**: `pkg/kv/kvclient/kvcoord/txn_interceptor_pipeliner.go`

Enables **write pipelining**—sending writes without waiting for acknowledgment.

```go
// Pipeline writes when safe
if canPipeline(req) {
    // Send without waiting
    return tc.wrapped.Send(ctx, ba)
}
// Otherwise, wait for previous writes
tc.waitForPipelinedWrites(ctx)
```

**Trade-off**: Pipelining reduces latency but complicates error handling. If a pipelined write fails, subsequent operations must be aborted.

#### 4. Span Refresher
**File**: `pkg/kv/kvclient/kvcoord/txn_interceptor_span_refresher.go`

Handles **read refresh**—an optimization for pushed transactions.

When a transaction is pushed to a higher timestamp (due to conflicts), it can often continue by verifying its reads are still valid at the new timestamp.

```go
// If pushed, try to refresh reads
if txn.WriteTimestamp != txn.ReadTimestamp {
    if canRefresh(readSpans) {
        // Continue at higher timestamp
        txn.ReadTimestamp = txn.WriteTimestamp
    } else {
        // Must retry from beginning
        return TransactionRetryError
    }
}
```

#### 5. Committer
**File**: `pkg/kv/kvclient/kvcoord/txn_interceptor_committer.go`

Manages the commit protocol, including **parallel commits**.

Traditional commit:
1. Write all intents
2. Write transaction record as COMMITTED
3. Resolve intents

Parallel commit:
1. Write all intents AND transaction record in one batch
2. Transaction implicitly committed when intents + record durable
3. Resolve intents asynchronously

```go
// Parallel commit: include EndTxn with writes
if canParallelCommit(ba) {
    ba.Requests = append(ba.Requests, endTxnReq)
    // All writes + commit in one round trip
}
```

#### 6. Metrics
Collects transaction statistics (duration, restarts, etc.).

#### 7. Lock Footprint
Tracks the set of keys locked by the transaction for conflict detection.

## Request Flow

Let's trace a write operation through the system:

```
1. Txn.Put(key, value)
   └─ TxnCoordSender.Send(BatchRequest)
      └─ Interceptor stack processes request
         └─ DistSender routes to range
            └─ Replica evaluates command
               └─ MVCC write creates intent
```

### Write Intents

A write doesn't immediately commit. Instead, it creates an **intent**:

```go
// pkg/storage/mvcc.go:1931-1970
func MVCCPut(ctx context.Context, rw ReadWriter, ...) error {
    // Write intent (provisional value)
    // Links to transaction record
}
```

Intent structure:
- **Key**: The written key
- **Value**: The new value
- **TxnMeta**: Transaction ID, timestamp, epoch

Intents are resolved (committed or aborted) when the transaction completes.

## Conflict Resolution

### Write-Write Conflicts

Two transactions writing the same key:

1. **First writer wins**: Creates intent
2. **Second writer detects intent**: Checks transaction status
   - If PENDING: Push or wait
   - If COMMITTED: Abort and retry
   - If ABORTED: Clean up and proceed

### Read-Write Conflicts

A transaction reads a key with a newer intent:

1. **Detect intent at higher timestamp**
2. **Check transaction status**
   - If COMMITTED: Read at intent timestamp
   - If PENDING/ABORTED: Read older version

### Pushing Transactions

When conflicts occur, CockroachDB tries to **push** rather than abort:

```go
// Push the timestamp
if canPushTimestamp(txn, conflicting) {
    txn.WriteTimestamp = conflicting.WriteTimestamp + 1
    // Continue with higher timestamp
}

// Push the abort
if shouldAbort(txn, conflicting) {
    return AbortError
}
```

**Priority rules**: Transactions have priorities. Higher priority wins conflicts.

## Automatic Retry

When a transaction must restart, CockroachDB can retry automatically:

```go
// pkg/sql/conn_executor.go
for {
    err := txn.Run(ctx, func(txn *kv.Txn) error {
        // Execute statements from buffer
        return nil
    })
    if !isRetryable(err) {
        return err
    }
    // Retry with new timestamp
}
```

**Requirements for automatic retry**:
- Error is retriable (e.g., `TransactionRetryError`)
- Transaction is implicit (no explicit BEGIN)
- All statements are buffered

## Timestamp Management

### Hybrid Logical Clocks (HLC)

CockroachDB uses HLC for timestamps:

```go
// pkg/util/hlc/timestamp.go
type Timestamp struct {
    WallTime int64  // Physical time (nanoseconds)
    Logical  int32  // Logical counter for same wall time
}
```

HLCs provide:
- **Causality**: If A → B, then ts(A) < ts(B)
- **Loose synchronization**: Works with clock skew
- **Total ordering**: All timestamps are comparable

### Uncertainty Interval

Distributed systems have clock skew. CockroachDB handles this with **uncertainty intervals**:

```go
// Uncertainty window: [txn.Timestamp, txn.MaxTimestamp]
if valueTimestamp > txn.Timestamp && valueTimestamp < txn.MaxTimestamp {
    // Value might be concurrent—uncertain!
    // Restart transaction with higher timestamp
}
```

This ensures serializable semantics despite clock skew.

## Transaction Record

Each transaction has a persistent record:

```proto
// pkg/roachpb/data.proto
message TransactionRecord {
    TransactionStatus status = 1;  // PENDING, COMMITTED, ABORTED
    Timestamp timestamp = 2;
    repeated Span intents = 3;
    // ...
}
```

The record is stored at `txn.Key` (usually the first write's key).

### Status Transitions

```
PENDING → COMMITTED (success)
PENDING → ABORTED (failure or push)
STAGING → COMMITTED (parallel commit)
```

## Performance Optimizations

### Write Pipelining

Without pipelining:
```
Write A → Wait → Write B → Wait → Write C → Wait → Commit
```

With pipelining:
```
Write A → Write B → Write C → Commit
   └─ Results arrive asynchronously
```

**Latency reduction**: N writes go from O(N × RTT) to O(1 × RTT).

### Parallel Commits

Traditional:
```
[Write intents] → [Write COMMITTED record] → [Resolve intents]
        RTT 1              RTT 2                 RTT 3+
```

Parallel:
```
[Write intents + STAGING record] → [Async resolve intents]
              RTT 1                       Background
```

**Latency reduction**: Commits complete in one round trip.

### Refresh Spans

When pushed, instead of aborting:

```go
// Track all reads
refreshSpans := []roachpb.Span{...}

// On push, verify reads still valid
for _, span := range refreshSpans {
    if hasNewerWrite(span, newTimestamp) {
        return RetryError  // Must abort
    }
}
// Continue at higher timestamp
```

**Benefit**: Many pushes become free (no actual conflict).

## Example: Complete Transaction

```go
// Application code
txn, err := db.NewTxn(ctx, "transfer")
defer txn.Cleanup(ctx)

// Read balances
fromBal, _ := txn.Get(ctx, fromKey)
toBal, _ := txn.Get(ctx, toKey)

// Update balances
txn.Put(ctx, fromKey, fromBal - amount)
txn.Put(ctx, toKey, toBal + amount)

// Commit
err = txn.Commit(ctx)
```

Behind the scenes:
1. **Get**: Read at transaction timestamp, track read spans
2. **Put**: Create intents, pipeline writes
3. **Commit**: Parallel commit with STAGING record
4. **Background**: Resolve intents asynchronously

## Key Takeaways

1. **Interceptor stack enables modularity**: Each concern (heartbeat, pipeline, refresh) is isolated.

2. **Intents defer commit decision**: Writes are provisional until commit.

3. **Pushing beats aborting**: Most conflicts resolve with timestamp pushes.

4. **Parallel commits reduce latency**: Writes and commit in one round trip.

5. **Refresh spans avoid retries**: Pushed transactions often continue without retry.

## What's Next

In Part 4, we'll explore the design patterns that make this complexity manageable—the interceptor pattern, factory pattern, and others that appear throughout CockroachDB.

---

## Further Reading

- [Transaction Layer Architecture](https://www.cockroachlabs.com/docs/stable/architecture/transaction-layer.html)
- [Serializable Snapshot Isolation](https://www.cockroachlabs.com/blog/serializable-lockless-distributed-isolation-cockroachdb/)
- [Parallel Commits](https://www.cockroachlabs.com/blog/parallel-commits/)

---

*This is Part 3 of the "Understanding CockroachDB Internals" series. Continue to Part 4: Patterns and Practices.*

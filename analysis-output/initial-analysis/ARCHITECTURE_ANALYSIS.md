# CockroachDB Architecture Analysis

## Executive Summary

CockroachDB implements a sophisticated distributed SQL database with a layered architecture:
- **SQL Layer** → **KV (Key-Value) Layer** → **Storage Layer** (RocksDB/Pebble)

The architecture emphasizes composability through interfaces, interceptor chains, and factory patterns, with comprehensive cross-cutting concerns for logging, metrics, tracing, and error handling.

---

## 1. LAYERED ARCHITECTURE

### 1.1 SQL Layer (pkg/sql/)
**Purpose**: PostgreSQL-compatible SQL query processing

**Key Entry Points**:
- File: `/home/user/cockroach/pkg/sql/conn_executor.go` (Line 1529)
- Type: `connExecutor struct`
- Responsibilities:
  - Parse SQL statements
  - Plan queries using optimizer
  - Execute plan nodes
  - Manage transaction state via FSM (Finite State Machine)
  - Handle prepared statements and caching

**Key Components**:
```
connExecutor (Line 1529)
├── stmtBuf: Statement buffer
├── clientComm: ClientComm interface (conn_io.go:733)
├── machine: FSM for state transitions
├── state: Transaction state (txnState)
├── extraTxnState: Additional transaction scoping
└── mon: Memory monitor for resource tracking
```

**Architecture Pattern - State Machine**:
- File: `/home/user/cockroach/pkg/util/fsm/` - Finite state machine implementation
- Used to manage connection and transaction lifecycle
- States: Open, Aborted, CommitWait, InternalError

### 1.2 KV Layer (pkg/kv/ and pkg/kv/kvclient/kvcoord/)
**Purpose**: Distributed transaction coordination and key-value operations

**Core Interfaces**:

1. **Sender Interface** (pkg/kv/sender.go:53)
   ```go
   type Sender interface {
       Send(context.Context, *kvpb.BatchRequest) (*kvpb.BatchResponse, *kvpb.Error)
   }
   ```
   - Implemented by: Txn, TxnCoordSender, DistSender, Store, Replica
   - Provides a uniform interface for request routing

2. **TxnSender Interface** (pkg/kv/sender.go:95)
   - Extends Sender
   - Manages transaction metadata propagation
   - Leaf vs Root transaction coordination

**Key Types**:

- **Txn** (pkg/kv/txn.go:73)
  - Client-side transaction wrapper
  - Creates batches and manages retries
  - Coordinates with TxnCoordSender

- **TxnCoordSender** (pkg/kv/kvclient/kvcoord/txn_coord_sender.go:113)
  - Root transaction coordinator
  - Manages transaction heartbeating
  - Accumulates lock spans
  - Handles transaction state machine (txnPending → txnFinalized)

- **DistSender** (pkg/kv/kvclient/kvcoord/dist_sender.go)
  - Routes batch requests to correct range replicas
  - Handles range cache lookups
  - Implements circuit breaker for fault tolerance
  - Metrics: batch counting, RPC tracking, cross-region/zone monitoring

### 1.3 Storage Layer (pkg/kv/kvserver/)
**Purpose**: Replica management, Raft consensus, and command execution

**Key Components**:

1. **Store** (pkg/kv/kvserver/store.go:885)
   ```
   Type: Store struct
   Contains:
   - engines: Multiple storage engine instances
   - replicas: Map of ranges by key
   - allocator: Replica placement decisions
   - Multiple queues: replicateQueue, splitQueue, mergeQueue, etc.
   - tsCache: Timestamp cache for conflict detection
   - metrics: StoreMetrics
   ```

2. **Replica** (pkg/kv/kvserver/replica.go:353)
   ```
   Type: Replica struct
   Contains:
   - RangeID: Unique range identifier
   - replicaID: Replica position within Raft group
   - store: Reference to parent Store
   - readOnlyCmdMu: RWMutex for read-only command isolation
   - raftMu: Protects Raft processing
   - mu: Main replica mutex (complex locking hierarchy)
   - isInitialized: Tracks replica readiness
   - flowControlV2: RACv2 flow control processor
   ```

3. **Command Pattern** (pkg/kv/kvserver/batcheval/command.go:51)
   ```go
   type Command struct {
       DeclareKeys DeclareKeysFunc  // Declare spans touched
       EvalRW      func(...) Result  // Read-write evaluation
       EvalRO      func(...) Result  // Read-only evaluation
   }
   ```
   - Registry pattern for method dispatch
   - ~60+ commands registered (Get, Put, Scan, etc.)
   - Enables pluggable command implementations

---

## 2. KEY ARCHITECTURAL ABSTRACTIONS

### 2.1 Request Path Flow

```
Client
  ↓
pgwire Protocol Handler (pgwire/conn.go)
  ↓
connExecutor (conn_executor.go) - SQL Planning & Execution
  ↓
kv.DB / kv.Txn (txn.go)
  ↓
TxnCoordSender (txn_coord_sender.go) - Transaction Coordination
  ↓
Interceptor Chain (txn_interceptor_*.go) - Cross-cutting concerns
  ├─ txnHeartbeater
  ├─ txnCommitter
  ├─ txnMetricRecorder
  ├─ txnLockGatekeeper
  └─ (others)
  ↓
DistSender (dist_sender.go) - Range Routing
  ↓
Transport Layer (transport.go) - RPC Communication
  ↓
Store.Send() (kvserver/store.go)
  ↓
concurrency.Manager (concurrency/concurrency_manager.go)
  ↓
Replica.Propose() → Raft
  ↓
batcheval.Command Evaluation (batcheval/command.go)
  ↓
Storage Engine (RocksDB/Pebble)
```

### 2.2 Interceptor Chain Pattern

**File**: `/home/user/cockroach/pkg/kv/kvclient/kvcoord/txn_coord_sender.go:187`

```go
type txnInterceptor interface {
    lockedSender
    setWrapped(wrapped lockedSender)
    populateLeafInputState(...)
    initializeLeaf(...)
    populateLeafFinalState(...)
    importLeafFinalState(...)
    epochBumpedLocked()
    createSavepointLocked(...)
    rollbackToSavepointLocked(...)
    releaseSavepointLocked(...)
    closeLocked()
}
```

**Implementations**:
1. `txnHeartbeater` - Maintains transaction liveness
2. `txnCommitter` - Handles 2PC protocol
3. `txnMetricRecorder` - Tracks transaction metrics
4. `txnLockGatekeeper` - Manages lock acquisition
5. `txnInterceptorPipeliner` - Request pipelining
6. `txnInterceptorSpanRefresher` - Timestamp conflict resolution
7. `txnInterceptorWriteBuffer` - Batches writes

**Pattern Benefits**:
- Separation of concerns
- Easy to add new transaction behaviors
- Order of interceptors matters (specified explicitly in `newRootTxnCoordSender`)

---

## 3. DESIGN PATTERNS IDENTIFIED

### 3.1 Factory Pattern

**SQL Optimizer Factory** (pkg/sql/opt/exec/factory.go)
- File: `/home/user/cockroach/pkg/sql/opt/exec/factory.go`
- Interface-based factory for creating execution operators
- Multiple implementations:
  - `execbuilder.Factory` - Production execution
  - `explain/explain_factory.go` - Explain statement output
  - `explain/plan_gist_factory.go` - Plan gist generation

**TxnCoordSender Factory** (pkg/kv/kvclient/kvcoord/txn_coord_sender_factory.go:59)
- Creates root and leaf transaction coordinators
- Provides centralized configuration management
- Pre-allocates interceptor instances

**Descriptor Collection Factory** (pkg/sql/catalog/descs/factory.go)
- Creates and manages schema metadata collections
- Handles descriptor caching and lease management

### 3.2 Strategy Pattern

**Sender Chain**:
- Different senders implement same interface
- Swappable based on deployment (local, gRPC, tenant)
- Examples:
  - `TxnCoordSender` (transaction coordination)
  - `DistSender` (range routing)
  - `Store.Send()` (local evaluation)

**Storage Engine Strategy**:
- Pluggable storage backends through `storage.Engine` interface
- Currently: RocksDB/Pebble
- Allows testing with in-memory implementations

### 3.3 Command Pattern

**Batch Evaluation** (pkg/kv/kvserver/batcheval/command.go:51)
- Command registration at init time
- Commands encapsulate:
  - Key declaration (what spans they touch)
  - Evaluation logic (read-only vs read-write)
  - Post-evaluation side effects

**Example Registrations**:
- `cmd_get.go` - GetRequest evaluation
- `cmd_put.go` - PutRequest evaluation
- `cmd_scan.go` - ScanRequest evaluation
- ~60 registered commands total

### 3.4 Repository Pattern

**Descriptor Catalog** (pkg/sql/catalog/)
- Abstracts schema metadata access
- Supports multiple implementations:
  - Lease-based caching
  - Synthetic descriptors for testing
  - Cached vs direct access

**File Structure**:
- `/home/user/cockroach/pkg/sql/catalog/descs/` - Descriptor collection
- `/home/user/cockroach/pkg/sql/catalog/lease/` - Descriptor leasing
- Transactions interact through unified interface

### 3.5 Observer/Event Pattern

**Metrics System** (pkg/util/metric/)
- `metric.Counter` - Cumulative counts
- `metric.Histogram` - Distribution tracking
- `metric.Gauge` - Point-in-time values
- `aggmetric.*` - Aggregated metrics across tenants

**Example Metrics** (from pkg/sql/server.go:585+):
```
- SQLExecLatency: Histogram of SQL execution times
- DistSQLSelectCount: Counter of distributed SQL selects
- TxnRetryCount: Counter of transaction retries
- SQLTxnsOpen: Gauge of open transactions
```

**Cross-Cutting Metrics**:
- DistSender metrics (batches, RPCs, bytes)
- Store metrics (replica counts, queue lengths)
- RPC metrics (sent, received, errors)

### 3.6 Decorator Pattern

**ClientComm Interface** (pkg/sql/conn_io.go:733)
- Multiple implementations providing result communication
- `pgwire.conn` - PostgreSQL wire protocol
- Buffers and flushes results to client
- Handles different data types and encoding

**Tracing Integration**:
- Wraps operations with OpenTelemetry spans
- Example: `/home/user/cockroach/pkg/util/tracing/`

---

## 4. CROSS-CUTTING CONCERNS

### 4.1 Logging

**Location**: `/home/user/cockroach/pkg/util/log/`

**Architecture**:
- `log.{Info,Warn,Error,KvExec}` - Severity levels
- Structured logging with `logpb` proto definitions
- Redaction support for sensitive data
- Log tag propagation through context

**Usage Patterns**:
```go
log.KvExec.Infof(ctx, "replica %s: applying batch %v", r.String(), batch)
```

**Key Integration Points**:
- SQL: `/home/user/cockroach/pkg/sql/conn_executor.go` - Logs statements
- KV: `/home/user/cockroach/pkg/kv/kvclient/kvcoord/dist_sender.go` - Logs routing decisions
- Storage: `/home/user/cockroach/pkg/kv/kvserver/replica.go` - Logs Raft state changes

### 4.2 Metrics and Observability

**Registry Pattern**:
- File: `/home/user/cockroach/pkg/server/server.go:272-279`
- Three registries:
  - `nodeRegistry` - Storage/KV layer metrics
  - `appRegistry` - Application metrics (SQL, transactions)
  - `sysRegistry` - Process-level metrics

**Metric Types**:

1. **Counters** (incremental-only):
   - `distsender.batches` - Batches sent
   - `sql.exec.latency` - Query execution time
   - `txn.retry` - Transaction retries

2. **Histograms** (distributions):
   - `distsender.rpc.sent` - RPC latency
   - `sql.exec.latency.consisten` - Consistent read latency

3. **Gauges** (point-in-time):
   - `sql.txns.open` - Currently open transactions
   - `sql.active_statements` - Running queries

**Aggregated Metrics**:
- `aggmetric.*` - Tenant-aware aggregation
- Per-tenant breakdown of resource usage

### 4.3 Tracing (OpenTelemetry Integration)

**Location**: `/home/user/cockroach/pkg/util/tracing/`

**Architecture**:
- Ambient context carries tracer
- Spans created at layer boundaries
- Attributes attached: query, user, operation type
- Integration: File `/home/user/cockroach/pkg/kv/kvclient/kvcoord/txn_coord_sender.go:30` imports OTEL

**Usage**:
```go
go.opentelemetry.io/otel/attribute
// Span operations automatically recorded
```

### 4.4 Error Handling

**Framework**: `github.com/cockroachdb/errors`

**Hierarchy**:
1. **PgError** - PostgreSQL-compatible errors with error codes
   - Located: `/home/user/cockroach/pkg/sql/pgwire/pgerror/`
   - Includes SQLSTATE codes for client compatibility

2. **Network Errors** - gRPC/RPC failures with retry logic
   - Auto-retried with exponential backoff
   - Handled in: `dist_sender.go`

3. **Transaction Errors**:
   - `TransactionRetryWithProtoRefreshError` - Retriable
   - `TransactionAbortedError` - Requires new TxnCoordSender
   - Handled in: `txn_coord_sender.go:56-75`

4. **Concurrency Errors**:
   - `WriteIntentError` - Lock conflicts
   - Handled via lock table waiter

**Error Propagation**:
- Errors wrapped at layer boundaries
- Context preserved for debugging
- Structured error information in responses

### 4.5 Resource Management

**Memory Accounting**:
- File: `/home/user/cockroach/pkg/util/mon/` - Memory monitoring
- `mon.BytesMonitor` tracks memory per session/transaction
- Memory limits enforced with quota reservations

**Transaction Resource Limits**:
- `server.max_open_transactions_per_gateway` - Limit open txns
- `sql.defaults.statement_timeout` - Statement timeout
- Guardrails for row read/write limits

**File**: `/home/user/cockroach/pkg/sql/conn_executor.go:101-131` registers settings

### 4.6 Context Management

**Ambient Context Pattern**:
- File: `/home/user/cockroach/pkg/server/server.go:286-299`
- Log tags for node ID, tenant ID attached early
- Propagated through all context.Context values
- Enables dynamic log tag updates via `base.NodeIDContainer`

---

## 5. REQUEST FLOW EXAMPLE: SELECT Query

```
User sends: SELECT * FROM users
    ↓
pgwire.Conn.handleSimpleQuery()
    ↓
connExecutor.ExecStmt()  [conn_executor.go:1529]
    - Parse SQL statement
    - Switch to txn.OPEN state
    - Create planner with schema metadata
    ↓
planner.Execute()
    - Query optimization via ORMOptimizer
    - Build execution plan
    - Return plan tree of operators
    ↓
planNode.Next() (for each row)
    - Leaf operations: TableScan, Scan
    - Calls DistSQL for distributed operations
    ↓
DistSQLPlanner.Plan()
    - Determines if query can run locally or needs distribution
    - Creates flow spec
    ↓
kv.DB.Scan() → Txn.Scan()
    - Creates ScanRequest batch
    ↓
TxnCoordSender.Send()
    - Passes through interceptor chain:
        1. txnMetricRecorder (record start time)
        2. txnHeartbeater (ensure txn liveness)
        3. txnLockGatekeeper (check lock eligibility)
    ↓
DistSender.Send()
    - Look up range containing keys
    - Create partial batches per range
    ↓
Transport.Send() → gRPC
    ↓
Store.Send() [kvserver/store.go]
    - Find replica for range
    ↓
concurrency.Manager.maybeInterceptReq()
    - Acquire latches (sequence operations)
    - Check lock table for conflicts
    ↓
Replica.Propose() → Raft
    - Read-only optimization: bypass Raft if possible
    - Apply through replicated state machine
    ↓
batcheval.EvalRO() [cmd_scan.go]
    - Declare keys that will be read
    - Scan storage engine
    - Return rows
    ↓
Response propagates back through layers
    ↓
ClientComm.CreateStatementResult()
    - Package rows into result set
    ↓
pgwire.Conn.Flush()
    - Send result to client
```

---

## 6. SPECIFIC FILE REFERENCES AND LINE NUMBERS

### SQL Layer Entry Points
- **`/home/user/cockroach/pkg/sql/conn_executor.go:1529`** - connExecutor struct definition
- **`/home/user/cockroach/pkg/sql/conn_io.go:733`** - ClientComm interface
- **`/home/user/cockroach/pkg/sql/conn_executor.go:101-131`** - Settings registration

### KV Layer Core
- **`/home/user/cockroach/pkg/kv/sender.go:53`** - Sender interface (line 53)
- **`/home/user/cockroach/pkg/kv/sender.go:95`** - TxnSender interface (line 95)
- **`/home/user/cockroach/pkg/kv/txn.go:73`** - Txn struct (line 73)
- **`/home/user/cockroach/pkg/kv/kvclient/kvcoord/txn_coord_sender.go:113`** - TxnCoordSender struct
- **`/home/user/cockroach/pkg/kv/kvclient/kvcoord/txn_coord_sender.go:187`** - txnInterceptor interface
- **`/home/user/cockroach/pkg/kv/kvclient/kvcoord/txn_coord_sender.go:236-285`** - Interceptor chain construction

### Storage Layer
- **`/home/user/cockroach/pkg/kv/kvserver/replica.go:353`** - Replica struct (line 353)
- **`/home/user/cockroach/pkg/kv/kvserver/store.go:885`** - Store struct (line 885)
- **`/home/user/cockroach/pkg/kv/kvserver/batcheval/command.go:51`** - Command struct and registry
- **`/home/user/cockroach/pkg/kv/kvserver/batcheval/command.go:78-100`** - Command registration functions

### Server & Configuration
- **`/home/user/cockroach/pkg/cmd/cockroach/main.go`** - Binary entry point
- **`/home/user/cockroach/pkg/server/server.go:250+`** - Server initialization
- **`/home/user/cockroach/pkg/server/server.go:272-279`** - Registry setup

### Patterns & Infrastructure
- **`/home/user/cockroach/pkg/sql/opt/exec/factory.go`** - Execution factory pattern
- **`/home/user/cockroach/pkg/kv/kvclient/kvcoord/dist_sender.go`** - DistSender with metrics
- **`/home/user/cockroach/pkg/util/log/`** - Logging infrastructure
- **`/home/user/cockroach/pkg/util/metric/`** - Metrics infrastructure
- **`/home/user/cockroach/pkg/util/mon/`** - Memory monitoring

### Architecture Documentation
- **`/home/user/cockroach/docs/design.md`** - High-level architecture document
- **`/home/user/cockroach/pkg/kv/doc.go`** - KV layer documentation (line 1-119)
- **`/home/user/cockroach/pkg/kv/kvserver/doc.go`** - Storage layer documentation
- **`/home/user/cockroach/pkg/kv/kvclient/kvcoord/doc.go`** - KV coordinator documentation

---

## 7. TRADEOFFS AND DESIGN DECISIONS

### 7.1 Complexity vs Flexibility
- **Interceptor Chain**: Adds complexity but enables clean separation of transaction concerns
  - Trade-off: Order of interceptors matters; requires careful management
  - Benefit: New transaction behaviors can be added without modifying existing code

### 7.2 Locking Hierarchy
- **Strict Lock Ordering** in Replica (line 970-1040 in store.go):
  - Required order: baseQueue.mu < Replica.raftMu < Replica.readOnlyCmdMu < Store.mu
  - Trade-off: Prevents deadlocks but makes code harder to understand
  - Benefit: Enables high concurrency with consistent ordering

### 7.3 Read Optimization
- **Bypass Raft for Read-Only Transactions**:
  - Trade-off: Requires careful timestamp handling (uncertainty intervals)
  - Benefit: Dramatically reduces read latency

### 7.4 Memory Accounting
- **Per-Session and Per-Transaction Tracking**:
  - split between sessionMon and txn-specific mon (conn_executor.go:1542-1560)
  - Trade-off: Additional memory overhead
  - Benefit: Separate statistics for session vs transaction resource usage

### 7.5 Command Registry
- **Static Registration Pattern**:
  - All commands registered at init time
  - Trade-off: No dynamic command registration
  - Benefit: Performance (array lookup in O(1)), type safety

---

## 8. KEY ARCHITECTURAL INSIGHTS

1. **Layering is Strict**: Each layer has well-defined interfaces (Sender, ClientComm, etc.)

2. **Interface-Based Design**: Almost all interactions use interfaces, enabling:
   - Testing with mocks
   - Multiple implementations
   - Clear contracts

3. **Interceptor Pattern is Ubiquitous**:
   - Used in transaction coordination
   - Enables modular transaction handling
   - Order matters and is explicit

4. **Metrics-First Design**:
   - Metrics at every layer boundary
   - Histograms track latencies
   - Per-tenant aggregation for multi-tenancy

5. **Context Propagation**:
   - Ambient context carries tracer, logger, tags
   - Enables correlation across distributed operations
   - Dynamic tag updates via atomic containers

6. **Careful Concurrency Management**:
   - Multiple synchronization mechanisms:
     - Mutexes for critical sections
     - Atomic values for lock-free reads
     - SpanSets for latch-based synchronization
   - Well-documented lock ordering prevents deadlocks

7. **Memory Safety at Scale**:
   - Memory monitors per connection/transaction
   - Prevents OOM from unbounded operations
   - Guardrails for rowcount limits

---

## CONCLUSION

CockroachDB's architecture demonstrates sophisticated software engineering:

- **Separation of Concerns**: Clear layering with focused responsibilities
- **Composability**: Interfaces and factories enable flexible combinations
- **Observability**: Comprehensive metrics, logging, and tracing at all levels
- **Reliability**: Careful error handling, retry logic, and resource limits
- **Performance**: Read optimization, async operations, parallel processing

The codebase uses well-established patterns (Factory, Strategy, Command, Decorator) applied thoughtfully to distributed systems challenges.


# Deep Dive: CockroachDB SQL Layer

**Series:** Understanding CockroachDB Internals (Part 2 of 6)
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## What You'll Learn

- How SQL queries are parsed into an AST
- The Cascades-style optimizer and memo structure
- Row-based vs columnar query execution
- How schema changes work online without blocking reads

---

## Introduction

The SQL layer is CockroachDB's user-facing interface—it's where PostgreSQL compatibility lives. With approximately 2,000 files in `pkg/sql/`, it's also the largest layer. In this post, we'll trace a query from text to execution, understanding each transformation along the way.

## The Query Lifecycle

Every SQL query follows this path:

```
SQL Text → Parser → AST → Opt Builder → Memo →
Optimizer → OptExpr → Exec Builder → Plan →
Executor → Results
```

Let's explore each stage.

## Parsing: Text to AST

The parser transforms SQL text into an Abstract Syntax Tree (AST). CockroachDB uses a Yacc-based parser, originally derived from Vitess.

**Entry Point**: [`pkg/sql/parser/parse.go:88`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/parser/parse.go#L88)

```go
// Parser struct wraps the scanning and parsing machinery
type Parser struct {
    scanner    scanner.SQLScanner
    lexer      lexer
    parserImpl sqlParserImpl
    tokBuf     [8]sqlSymType  // Token lookahead buffer
}

func (p *Parser) Parse(sql string) (statements.Statements, error)
```

### Design Decisions

**Token buffering**: The 8-token lookahead buffer enables efficient parsing without backtracking.

**Multiple statements**: A single `Parse` call can return multiple statements (`SELECT ...; INSERT ...;`).

**Comment preservation**: Optional, for IDE/tooling support.

The output is a tree of `tree.Statement` nodes defined in `pkg/sql/sem/tree/`.

## Building the Memo: AST to Optimizer Representation

The optimizer doesn't work directly with the AST. Instead, the **optbuilder** transforms it into a memo—a forest of logically equivalent expressions.

**Entry Point**: [`pkg/sql/opt/optbuilder/builder.go:60-106`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/opt/optbuilder/builder.go#L60)

```go
type Builder struct {
    factory *norm.Factory
    stmt    tree.Statement
    catalog cat.Catalog
    evalCtx *eval.Context
    // ... 100+ fields for context
}

func (b *Builder) Build() (err error)
```

The builder performs:
- **Name resolution**: What table is `users`? Which column is `id`?
- **Type checking**: Is `5 + 'hello'` valid?
- **Semantic analysis**: Do we have permission? Does the table exist?

## The Memo: Why It Matters

The memo is CockroachDB's secret weapon for optimization. It stores logically equivalent expressions without duplication.

**Location**: [`pkg/sql/opt/memo/memo.go:116-150`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/opt/memo/memo.go#L116)

```go
type Memo struct {
    // Groups of equivalent expressions
    groups []group

    // Interning ensures unique expressions
    interner interner

    // Metadata about tables, columns, etc.
    metadata *opt.Metadata
}
```

### Expression Interning

Consider the query `SELECT * FROM a JOIN b JOIN c`. The optimizer explores:
- `(a JOIN b) JOIN c`
- `a JOIN (b JOIN c)`
- `(b JOIN a) JOIN c`
- ... many more

Without interning, this creates exponential expression copies. With interning:

```go
// pkg/sql/opt/memo/interner.go
// Same expression → same pointer
expr1 == expr2  // Pointer comparison!
```

This enables O(1) equivalence checking—essential for exploring millions of plans.

## Optimization: Finding the Best Plan

The optimizer uses the Cascades framework: iteratively apply transformation rules to expand the memo, then select the lowest-cost plan.

**Entry Point**: [`pkg/sql/opt/xform/optimizer.go:250`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/opt/xform/optimizer.go#L250)

```go
type Optimizer struct {
    mem      *memo.Memo      // Expression forest
    f        norm.Factory    // Normalization
    explorer explorer        // Rule exploration
    coster   Coster          // Cost estimation
}

func (o *Optimizer) Optimize() (_ opt.Expr, err error)
```

### Two-Phase Optimization

**Phase 1: Normalization** (always applied)
- Predicate push-down
- Constant folding
- Redundancy elimination

**Phase 2: Exploration** (cost-based)
- Join reordering
- Index selection
- Distributed execution strategies

### Cost Model

The coster estimates CPU and I/O costs using:
- Table statistics (row counts, distinct values)
- Selectivity estimates
- Memory requirements

```go
// From pkg/sql/opt/xform/coster.go
type Cost struct {
    // CPU and I/O components
    cpuCost float64
    ioCost  float64
}
```

## Execution: Running the Plan

Once optimized, the plan is built into an executable form.

**Exec Builder**: [`pkg/sql/opt/exec/execbuilder/builder.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/opt/exec/execbuilder/builder.go#L57)

CockroachDB has two execution engines:

### Row-Based Execution

Traditional tuple-at-a-time processing.

```go
// pkg/sql/execinfra/processorsbase.go:35-62
type Processor interface {
    OutputTypes() []*types.T
    Run(context.Context, RowReceiver)
    Close(context.Context)
}
```

**Use cases**: Complex expressions, UDFs, some aggregate operations.

### Columnar (Vectorized) Execution

Batch-oriented processing optimized for modern CPUs.

```go
// pkg/sql/colexecop/operator.go:20-51
type Operator interface {
    Init(ctx context.Context)
    Next() coldata.Batch  // Returns 1024-2048 rows at once
}
```

**Advantages**:
- Cache-friendly memory access
- SIMD opportunities
- Reduced interpretation overhead

**Implementation**: Template files (`.tmpl`) generate type-specialized code (`.eg.go`) for each data type:

```
// Separate implementations for:
int16, int32, int64, float32, float64,
decimal, bool, bytes, timestamp, interval, ...
```

## Example: Query Execution Flow

Let's trace `SELECT name FROM users WHERE id = 5`:

```
1. Parse
   └─ SelectStmt{From: "users", Where: id=5, Cols: [name]}

2. Build
   └─ Scan(users) → Filter(id=5) → Project(name)

3. Normalize
   └─ Scan(users, constraint: id=5) → Project(name)
      (Filter pushed into scan)

4. Explore
   ├─ Scan via primary key (cost: 1.0)
   └─ Scan via secondary index on id (cost: 1.2)
   → Select primary key scan

5. Execute
   └─ ColexecScan → ColexecProject → Results
```

## Schema Changes: Online DDL

CockroachDB performs schema changes online—without blocking reads or writes. This is handled by the **declarative schema changer**.

**Entry Point**: [`pkg/sql/schemachanger/scrun/scrun.go:43-108`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/schemachanger/scrun/scrun.go#L43)

### Element Model

Instead of imperative mutations, schema changes use declarative elements:

```go
// Each element has a status
type Element interface {
    // Column, Index, Constraint, etc.
}

type Status int
const (
    PUBLIC Status = iota
    WRITE_ONLY
    DELETE_ONLY
    BACKFILLING
    ABSENT
)
```

### 2-Version Invariant

The key constraint: **at most two schema versions exist simultaneously**.

For adding a column:
1. `ABSENT` → `DELETE_ONLY` (new writes include column)
2. `DELETE_ONLY` → `WRITE_ONLY` (all nodes write column)
3. `WRITE_ONLY` → `PUBLIC` (column visible to reads)

For dropping:
1. `PUBLIC` → `WRITE_ONLY`
2. `WRITE_ONLY` → `DELETE_ONLY`
3. `DELETE_ONLY` → `ABSENT`

Each transition is a separate transaction, ensuring readers never see data that old writers haven't written.

### Rules-Based Dependencies

The schema changer uses ~100+ rules to determine ordering:

```go
// "ColumnName must be ABSENT before Column is ABSENT"
// "Index backfill must complete before Index becomes PUBLIC"
```

Rules are expressed as predicates and evaluated by a mini relational engine.

## Session Management

The `connExecutor` coordinates SQL execution for a connection:

**Location**: [`pkg/sql/conn_executor.go:213-289`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/sql/conn_executor.go#L213)

```go
type connExecutor struct {
    server     *Server
    stmtBuf    *StmtBuf       // Incoming statements
    clientComm ClientComm     // Result delivery
    machine    fsm.Machine    // State machine
    state      txnState       // Transaction state

    // Memory accounting
    mon        *mon.BytesMonitor
}
```

### State Machine

The executor uses a state machine for transaction management:

```
NoTxn → Open → (Aborted | CommitWait) → NoTxn
```

States:
- **NoTxn**: Outside a transaction
- **Open**: Active transaction
- **Aborted**: Transaction failed, awaiting ROLLBACK
- **CommitWait**: Awaiting COMMIT confirmation

### Automatic Retry

When a transaction conflicts, CockroachDB can retry automatically:

1. Buffer all statements
2. On conflict, rewind to transaction start
3. Re-execute from buffer

This happens transparently to the client for retriable errors.

## Performance Considerations

### Optimizer Performance

- **Memo interning**: Avoids exponential memory growth
- **Cost pruning**: Stops exploring high-cost alternatives early
- **Group caching**: Reuses optimized results per group

### Execution Performance

- **Columnar batching**: 1024-2048 rows per batch
- **Type specialization**: No reflection in hot paths
- **Memory pooling**: Batch reuse avoids allocation

### Memory Management

```go
// Per-level monitors
sessionMon      // Prepared statements
txnMon          // Transaction state
resultMon       // Result sets
```

Each monitor enables:
- Resource accounting
- OOM prevention
- Memory-limited operations (sort, hash join)

## Key Takeaways

1. **The memo is crucial**: Expression interning enables exploring millions of plans without memory explosion.

2. **Columnar execution wins for analytics**: Batching + type specialization provides major speedups.

3. **Schema changes are declarative**: Rules-based dependencies ensure correctness during online DDL.

4. **Automatic retry is transparent**: Buffered statements enable seamless retry on conflicts.

5. **Memory is tracked per-level**: Monitors enable fine-grained resource management.

## Code Examples

### Parsing a Query
```go
parser := parser.Parser{}
stmts, err := parser.Parse("SELECT * FROM users WHERE id = $1")
// stmts[0] is a *tree.Select
```

### Building and Optimizing
```go
builder := optbuilder.New(ctx, &semaCtx, evalCtx, catalog, factory, stmt)
if err := builder.Build(); err != nil {
    return err
}
opt := xform.NewOptimizer(ctx, evalCtx, catalog)
optimizedExpr, err := opt.Optimize()
```

### Creating an Executor
```go
execBuilder := execbuilder.New(
    ctx, factory, optimizer, memo, catalog, evalCtx,
)
plan, err := execBuilder.Build()
```

## What's Next

In Part 3, we'll explore distributed transactions—how CockroachDB maintains serializable isolation across a cluster. We'll see the TxnCoordSender interceptor stack in action and understand the conflict resolution protocols.

---

## Further Reading

- [Query Optimizer Overview](https://github.com/cockroachdb/cockroach/blob/master/pkg/sql/opt/doc.go)
- [Vectorized Execution](https://www.cockroachlabs.com/blog/how-we-built-a-vectorized-execution-engine/)
- [Online Schema Changes](https://www.cockroachlabs.com/blog/online-schema-changes/)

---

*This is Part 2 of the "Understanding CockroachDB Internals" series. Continue to Part 3: Distributed Transactions.*

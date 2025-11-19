# Observability Deep Dive

**Series:** Understanding CockroachDB Internals (Part 5 of 6)
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## What You'll Learn

- CockroachDB's channel-based logging architecture
- Metrics collection and Prometheus export
- Distributed tracing with OpenTelemetry
- Debug endpoints for production troubleshooting

---

## Introduction

Operating a distributed database requires deep visibility into system behavior. CockroachDB provides comprehensive observability through structured logging, metrics, and distributed tracing. In this post, we'll explore how these systems work and how to use them effectively.

## Logging Architecture

CockroachDB's logging system is designed for operators managing large-scale deployments.

### Channel-Based Routing

**Location**: [`pkg/util/log/log_channels_generated.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/util/log/log_channels_generated.go)

Instead of a single log stream, messages route to specific **channels**:

| Channel | Purpose | Audience |
|---------|---------|----------|
| `DEV` | Development/debugging | Developers |
| `OPS` | Operational notices | Operators |
| `HEALTH` | Health status | Monitoring |
| `STORAGE` | Storage layer events | Storage engineers |
| `SESSIONS` | Session lifecycle | Security/audit |
| `SQL_SCHEMA` | Schema changes | DBAs |
| `SQL_PERF` | Query performance | DBAs |
| `TELEMETRY` | Telemetry data | Analytics |
| `PRIVILEGES` | Permission changes | Security |
| `SENSITIVE_ACCESS` | Sensitive data access | Compliance |

**Usage**:

```go
// Different channels for different audiences
log.Dev.Infof(ctx, "debug info for developers")
log.Ops.Warningf(ctx, "operational warning for operators")
log.Health.Infof(ctx, "health check passed")
```

### Severity Levels

Standard levels: `INFO`, `WARNING`, `ERROR`, `FATAL`

```go
log.Info(ctx, "normal operation")
log.Warningf(ctx, "potential issue: %v", err)
log.Errorf(ctx, "operation failed: %v", err)
log.Fatalf(ctx, "unrecoverable error: %v", err)  // Exits process
```

### Structured Logging

Events can be logged as structured JSON:

```go
// pkg/util/log/structured_v2.go
var STATEMENT_STATS = StructuredLogMeta{
    EventType: "stmt_stats",
    Version:   "0.1",
}

log.Structured(ctx, STATEMENT_STATS, statsPayload)
```

**Output**:
```json
{"EventType": "stmt_stats", "Version": "0.1", "Payload": {...}}
```

### V-Style Logging

For verbose debugging:

```go
// Only logs if verbosity >= 2
if log.V(2) {
    log.Infof(ctx, "detailed debug info")
}

// Or more efficient
if log.ExpensiveLogEnabled(ctx, 2) {
    expensiveMsg := computeExpensiveMessage()
    log.VInfof(ctx, 2, expensiveMsg)
}
```

Control with `--vmodule=file=level`:
```bash
cockroach start --vmodule=txn_coord_sender=3,replica=2
```

### Ambient Context

The `AmbientContext` pattern propagates logging context:

**Location**: [`pkg/util/log/ambient_context.go:50-115`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/util/log/ambient_context.go#L50)

```go
type AmbientContext struct {
    Tracer    *tracing.Tracer
    ServerIDs serverident.ServerIdentificationPayload
    tags      *logtags.Buffer
}

// Usage in servers
func (s *Server) processRequest(ctx context.Context, req Request) {
    ctx = s.AnnotateCtx(ctx)  // Add server tags
    // All logs now include server identification
    log.Infof(ctx, "processing request")
}
```

## Metrics System

CockroachDB exports Prometheus-compatible metrics for monitoring.

### Metric Types

**Location**: [`pkg/util/metric/metric.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/util/metric/metric.go#L31)

```go
// Counter: monotonically increasing
requestCount := metric.NewCounter(metadata)
requestCount.Inc(1)

// Gauge: point-in-time value
activeConns := metric.NewGauge(metadata)
activeConns.Update(42)

// Histogram: distribution
latency := metric.NewHistogram(metadata, buckets)
latency.RecordValue(duration.Milliseconds())
```

### Registry Pattern

Metrics are organized in registries:

```go
// pkg/util/metric/registry.go
type Registry struct {
    tracked map[string]Iterable
    labels  []labelPair
}

// Server-level registry
serverRegistry := metric.NewRegistry()
serverRegistry.AddMetric(requestCount)

// Sub-registries
serverRegistry.MustAdd("cr.node.%s", nodeRegistry)
```

### Labels

Metrics support labels for dimensions:

```go
// Per-application metrics
requestsByApp := metric.NewCounterVec(metadata, []string{"app"})
requestsByApp.With(prometheus.Labels{"app": "myapp"}).Inc()
```

**Cardinality limit**: 2,000 distinct label combinations to prevent memory explosion.

### Key Metrics

| Metric | Type | Purpose |
|--------|------|---------|
| `sql.query.count` | Counter | Total queries |
| `sql.exec.latency` | Histogram | Query latency |
| `ranges.unavailable` | Gauge | Unavailable ranges |
| `liveness.heartbeatfailures` | Counter | Heartbeat failures |
| `txn.commits` | Counter | Committed transactions |

### Prometheus Export

**Endpoint**: `/_status/vars`

```go
// pkg/util/metric/prometheus_exporter.go
type PrometheusExporter struct {
    families  map[string]*prometheusgo.MetricFamily
    selection map[string]struct{}
}
```

Prometheus scrape config:
```yaml
scrape_configs:
  - job_name: 'cockroachdb'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/_status/vars'
```

## Distributed Tracing

CockroachDB integrates with OpenTelemetry for distributed tracing.

### Tracer Configuration

**Location**: [`pkg/util/tracing/tracer.go:46-299`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/util/tracing/tracer.go#L46)

```go
// Cluster settings
SET CLUSTER SETTING trace.opentelemetry.collector = 'localhost:4317';
SET CLUSTER SETTING trace.zipkin.collector = 'localhost:9411';
```

**Environment variables**:
- `COCKROACH_OTLP_COLLECTOR`: OTEL collector address
- `COCKROACH_ZIPKIN`: Zipkin collector address

### Span Management

```go
// pkg/util/tracing/span.go
ctx, span := tracer.StartSpanCtx(ctx, "operationName")
defer span.Finish()

// Add tags
span.SetTag("txnID", txn.ID)

// Add events
span.RecordStructured(&eventPayload)
```

### Recording Modes

```go
const (
    RecordingOff       // No recording
    RecordingStructured // Structured events only
    RecordingVerbose   // Everything
)

span.SetRecording(tracingpb.RecordingVerbose)
```

### Context Propagation

Spans propagate through context:

```go
// pkg/util/tracing/context.go
span := tracing.SpanFromContext(ctx)
if span != nil {
    span.SetTag("key", value)
}

// Create child span
ctx, childSpan := span.Tracer().StartSpanCtx(ctx, "childOp")
```

### Trace Limits

To prevent unbounded growth:

```go
const (
    maxRecordedSpansPerTrace   = 1000
    maxLogBytesPerSpan        = 256 * 1024  // 256 KiB
    maxStructuredBytesPerSpan = 10 * 1024   // 10 KiB
    maxSpanRegistrySize       = 5000
)
```

## Debug Endpoints

CockroachDB exposes debug endpoints for troubleshooting.

### pprof Endpoints

**Location**: [`pkg/server/debug/server.go:91-143`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/server/debug/server.go#L91)

| Endpoint | Purpose |
|----------|---------|
| `/debug/pprof/profile` | CPU profile |
| `/debug/pprof/heap` | Heap profile |
| `/debug/pprof/goroutine` | Goroutine stacks |
| `/debug/pprof/trace` | Execution trace |

**Usage**:
```bash
# 30-second CPU profile
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof
```

### Active Traces

```sql
SET CLUSTER SETTING trace.span_registry.enabled = true;
```

View at: `/#/debug/tracez`

### Log Spy

Real-time log inspection:

**Location**: [`pkg/server/debug/logspy.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/server/debug/logspy.go#L30)

```bash
# Watch logs matching pattern
curl "http://localhost:8080/debug/logspy?grep=txn&duration=10s"
```

## Health Checks

**Location**: [`pkg/server/status/health_check.go`](https://github.com/cockroachdb/cockroach/blob/6ca473a526d30cccd2d55ea87a6bf4bd22db3e40/pkg/server/status/health_check.go#L18)

### Endpoints

- `/health`: Basic liveness check
- `/health?ready=1`: Readiness check (includes SQL listener)

### Tracked Metrics

```go
var trackedMetrics = map[string]threshold{
    "ranges.unavailable":          gaugeZero,
    "ranges.underreplicated":      gaugeZero,
    "liveness.heartbeatfailures":  counterZero,
    "round-trip-latency-p90":      {gauge: true, min: time.Second},
}
```

## Event Logging

Significant events are logged to `system.eventlog`:

```go
// pkg/util/log/event_log.go
log.EventLog(ctx, &eventpb.CreateTable{
    TableName: "users",
    // ...
})
```

**Query**:
```sql
SELECT * FROM system.eventlog
WHERE "eventType" = 'create_table'
ORDER BY timestamp DESC;
```

## Best Practices

### Operator Setup

1. **Configure log channels**:
   ```bash
   cockroach start \
     --log='file-defaults: {dir: /var/log/cockroach}' \
     --log='sinks: {file-groups: {ops: {channels: [OPS, HEALTH]}}}'
   ```

2. **Set up Prometheus**:
   ```yaml
   scrape_configs:
     - job_name: 'cockroachdb'
       scrape_interval: 10s
       static_configs:
         - targets: ['node1:8080', 'node2:8080', 'node3:8080']
   ```

3. **Configure tracing** (optional):
   ```sql
   SET CLUSTER SETTING trace.opentelemetry.collector = 'jaeger:4317';
   ```

### Debugging Production Issues

1. **Check health**: `/health?ready=1`
2. **Review metrics**: Look for anomalies in Grafana
3. **Enable verbose logging**: `--vmodule` for specific files
4. **Capture profiles**: CPU and memory profiles during issues
5. **Review traces**: For specific slow queries

### Performance Considerations

- **Logging overhead**: V-style logging with `ExpensiveLogEnabled` for expensive computations
- **Metrics cardinality**: Keep label cardinality under 2,000
- **Tracing cost**: Use `RecordingStructured` instead of `RecordingVerbose` in production

## Key Takeaways

1. **Channel-based logging targets audiences**: Operators see different logs than developers.

2. **Metrics are Prometheus-native**: Easy integration with standard monitoring tools.

3. **Tracing is built-in**: Enable OpenTelemetry to trace queries across nodes.

4. **Debug endpoints are powerful**: pprof, log spy, and active traces for troubleshooting.

5. **Health checks are granular**: Separate liveness and readiness checks.

## What's Next

In Part 6, we'll analyze CockroachDB's performance characteristics and identify optimization opportunities.

---

## Further Reading

- [Logging Configuration](https://www.cockroachlabs.com/docs/stable/configure-logs.html)
- [Monitoring and Alerting](https://www.cockroachlabs.com/docs/stable/monitoring-and-alerting.html)
- [Troubleshooting](https://www.cockroachlabs.com/docs/stable/troubleshooting-overview.html)

---

*This is Part 5 of the "Understanding CockroachDB Internals" series. Continue to Part 6: Performance Analysis.*

# CockroachDB Codebase Metrics Summary

**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`
**Analysis Date:** November 19, 2025

---

## Code Volume

| Metric | Count | Notes |
|--------|-------|-------|
| Total Go Files | 8,600 | Source + test files |
| Test Files | 2,873 | 33% of all Go files |
| Protocol Buffer Files | 171 | API and data definitions |
| Repository Size | 752 MB | Including dependencies |

## Package Distribution

### Primary Packages by File Count (Estimated)

| Package | Files | Purpose |
|---------|-------|---------|
| `pkg/sql/` | ~2,000 | SQL layer |
| `pkg/kv/` | ~500 | KV layer |
| `pkg/storage/` | ~200 | Storage engine |
| `pkg/server/` | ~150 | Server management |
| `pkg/util/` | ~400 | Utilities |
| `pkg/ccl/` | ~600 | Enterprise features |
| `pkg/testutils/` | ~200 | Test infrastructure |
| `pkg/cmd/` | ~300 | CLI tools |

## Testing Metrics

### Test Infrastructure Scale

| Type | Count | Location |
|------|-------|----------|
| Unit Test Files | 2,873 | `*_test.go` |
| Logic Test Configs | 23 | `pkg/sql/logictest/` |
| Testdata Directories | 203 | Various |
| Roachtest Operations | 40+ | `pkg/cmd/roachtest/operations/` |
| Test Utility Packages | 34 | `pkg/testutils/` |

### Test Configuration Types

```
Logic Test Configurations:
- local                    # Single-node
- fakedist                 # Simulated distributed
- 5node                    # 5-node cluster
- local-vec-off            # Vectorization disabled
- local-mixed-25.2         # Mixed-version
- local-mixed-25.3
- local-mixed-25.4
- multiregion-9node-3region-3azs
- ... 15 more configurations
```

## Build System Metrics

### Bazel Configuration

| Metric | Details |
|--------|---------|
| Workspace File | `WORKSPACE` |
| Root BUILD | `BUILD.bazel` |
| Dev Tool Version | 114 |
| CI Workflows | 10+ in `.github/workflows/` |

### Code Generation Targets

| Generator | Purpose |
|-----------|---------|
| Gazelle | BUILD.bazel files |
| protobuf | Protocol buffers |
| parser | SQL grammar |
| optgen | Optimizer rules |
| execgen | Execution operators |
| stringer | String methods |
| schemachanger | Schema operations |

## Architecture Metrics

### Layer Complexity

| Layer | Key File | Approx Lines | Complexity |
|-------|----------|--------------|------------|
| SQL Executor | `conn_executor.go` | 3,000+ | High |
| Optimizer | `optimizer.go` | 500+ | High |
| Transaction Coord | `txn_coord_sender.go` | 800+ | High |
| Replica | `replica.go` | 1,000+ | High |
| Store | `store.go` | 2,000+ | High |

### Design Pattern Usage

| Pattern | Occurrences | Primary Location |
|---------|-------------|------------------|
| Interceptor | 7 types | `pkg/kv/kvclient/kvcoord/` |
| Factory | 10+ | Throughout |
| Command | 60+ | `pkg/kv/kvserver/batcheval/` |
| Registry | 5+ | Settings, metrics, commands |
| Strategy | 10+ | Senders, executors |

## Observability Metrics

### Logging Infrastructure

| Component | Details |
|-----------|---------|
| Log Channels | 10+ (DEV, OPS, HEALTH, etc.) |
| Severity Levels | 5 (INFO, WARNING, ERROR, FATAL, PANIC) |
| Structured Events | Version-tracked JSON payloads |

### Metrics System

| Type | Limit/Details |
|------|---------------|
| Cardinality Limit | 2,000 distinct label combinations |
| Histogram Windows | 2 rolling windows |
| Label Types | App, DB, AppAndDB |
| Export Format | Prometheus-compatible |

### Tracing Constraints

| Constraint | Value |
|------------|-------|
| Max Spans per Trace | 1,000 |
| Max Log Bytes per Span | 256 KiB |
| Max Structured Bytes per Span | 10 KiB |
| Max Structured Bytes per Trace | 1 MiB |
| Max Active Root Spans | 5,000 |

## Deployment Metrics

### Container Configuration

| Setting | Value |
|---------|-------|
| Base Image | UBI 10 minimal |
| Exposed Ports | 26257 (gRPC), 8080 (HTTP) |
| Working Directory | `/cockroach/` |

### Kubernetes Defaults

| Setting | Value |
|---------|-------|
| Default Replicas | 3 |
| Max Unavailable | 1 |
| Anti-affinity | Preferred different nodes |

## Performance Characteristics

### Memory Management

| Monitor | Scope |
|---------|-------|
| Session Monitor | Per-connection |
| Transaction Monitor | Per-transaction |
| Result Monitor | Per-result set |
| Prepared Statement Monitor | Per-session cache |

### Query Processing

| Stage | Key Optimization |
|-------|------------------|
| Parsing | Token buffering (8 tokens) |
| Optimization | Memo interning, cost pruning |
| Execution | Columnar batching (1024-2048 rows) |

## Dependency Metrics

### Build Dependencies

| Category | Count/Details |
|----------|---------------|
| Go Modules | 100+ in go.mod |
| C Dependencies | Pebble, GEOS |
| Build Tool | Bazel |
| Cross-compilation | Linux, macOS, Windows |

### External Integrations

| Integration | Library |
|-------------|---------|
| OpenTelemetry | `go.opentelemetry.io/otel` |
| Prometheus | `github.com/prometheus/client_golang` |
| gRPC | `google.golang.org/grpc` |
| Pebble | Internal (storage engine) |

## CI/CD Metrics

### GitHub Actions Workflows

| Workflow | Timeout | Purpose |
|----------|---------|---------|
| Acceptance | 60 min | Acceptance tests |
| Lint | 120 min | Full linting |
| ORM Tests | 120 min | ORM compatibility |
| Unit Tests | Variable | Go unit tests |
| Microbenchmarks | Variable | Performance tracking |

### Release Process

| Stage | Automation |
|-------|------------|
| Version Update | Automated PR |
| Binary Validation | Platform-specific tests |
| Helm Chart Update | Automated |
| Homebrew Update | Automated |

---

## Summary

CockroachDB is a large, mature codebase with:
- **Strong test coverage** (33% test files)
- **Comprehensive observability** (metrics, logging, tracing)
- **Sophisticated build system** (Bazel + code generation)
- **Production-ready deployment** (Docker, Kubernetes, cloud)

The complexity is concentrated in core components (executor, optimizer, replica) while maintaining good separation between layers.

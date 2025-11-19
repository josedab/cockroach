# CockroachDB Repository Structure

**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Top-Level Directory Structure

```
cockroach/
├── pkg/                    # Main source code (8,600+ Go files)
├── build/                  # Build scripts and Docker configs
├── cloud/                  # Cloud deployment manifests
├── docs/                   # Technical documentation and RFCs
├── licenses/               # Third-party licenses
├── scripts/                # Utility scripts
├── c-deps/                 # C/C++ dependencies (Pebble, GEOS)
├── .github/                # GitHub Actions workflows
├── WORKSPACE               # Bazel workspace definition
├── BUILD.bazel             # Root BUILD file
├── dev                     # Development tool wrapper
├── go.mod                  # Go module definition
├── DEPS.bzl                # Bazel dependency definitions
└── CLAUDE.md               # AI assistant guidance
```

## Core Package Structure (`pkg/`)

### SQL Layer

```
pkg/sql/                           # SQL engine (~2,000 files)
├── parser/                        # SQL parser (Yacc-based)
│   ├── parse.go                   # Parser entry points
│   ├── lexer.go                   # SQL tokenization
│   └── sql.y                      # SQL grammar (generated)
├── sem/                           # Semantic analysis
│   ├── tree/                      # AST definitions
│   ├── builtins/                  # Built-in functions
│   └── eval/                      # Expression evaluation
├── opt/                           # Query optimizer
│   ├── memo/                      # Memo structure
│   ├── norm/                      # Normalization rules
│   ├── xform/                     # Exploration/transformation
│   ├── optbuilder/                # AST to memo builder
│   └── exec/execbuilder/          # Memo to execution plan
├── colexec/                       # Columnar execution
├── rowexec/                       # Row-based execution
├── execinfra/                     # Execution infrastructure
├── schemachanger/                 # Declarative schema changes
│   ├── scbuild/                   # DDL to element builder
│   ├── scplan/                    # Plan generation
│   ├── scexec/                    # Plan execution
│   └── scop/                      # Schema operations
├── catalog/                       # Schema metadata
├── pgwire/                        # PostgreSQL wire protocol
├── conn_executor.go               # Connection execution coordinator
└── logictest/                     # SQL logic tests
```

### KV Layer

```
pkg/kv/                            # Key-Value layer
├── kv.go                          # Package documentation
├── txn.go                         # Transaction interface
├── db.go                          # Database client
├── kvclient/                      # KV client libraries
│   ├── kvcoord/                   # Transaction coordination
│   │   ├── txn_coord_sender.go    # Main coordinator
│   │   ├── dist_sender.go         # Range routing
│   │   └── txn_interceptor_*.go   # Interceptor implementations
│   ├── rangecache/                # Range descriptor cache
│   └── kvstreamer/                # Streaming KV access
├── kvserver/                      # KV server (storage nodes)
│   ├── replica.go                 # Replica management
│   ├── store.go                   # Store management
│   ├── batcheval/                 # Command evaluation
│   ├── concurrency/               # Locking and latching
│   ├── raftlog/                   # Raft log handling
│   └── rangefeed/                 # Change data capture
├── kvpb/                          # KV protocol buffers
└── bulk/                          # Bulk operations
```

### Storage Layer

```
pkg/storage/                       # Storage engine interface
├── engine.go                      # Engine interface
├── pebble.go                      # Pebble implementation
├── mvcc.go                        # MVCC operations
├── mvcc_key.go                    # MVCC key encoding
├── sst_writer.go                  # SST file writing
└── intent_interleaving_iter.go    # Intent handling
```

### Server Components

```
pkg/server/                        # Server management
├── server.go                      # Main server struct
├── node.go                        # Node management
├── status/                        # Status endpoints
├── debug/                         # Debug endpoints
└── tenant*.go                     # Multi-tenancy support

pkg/cli/                           # Command-line interface
├── start.go                       # Server startup
├── flags.go                       # CLI flags
└── sql.go                         # SQL shell
```

### Infrastructure

```
pkg/util/                          # Shared utilities
├── log/                           # Logging framework
├── metric/                        # Metrics collection
├── tracing/                       # Distributed tracing
├── mon/                           # Memory monitoring
├── encoding/                      # Data encoding
├── syncutil/                      # Synchronization utilities
├── timeutil/                      # Time utilities
└── uuid/                          # UUID generation

pkg/settings/                      # Cluster settings
├── setting.go                     # Setting interface
├── bool.go                        # Boolean settings
├── int.go                         # Integer settings
└── cluster/                       # Cluster settings

pkg/roachpb/                       # Core protocol buffers
├── api.proto                      # API definitions
├── data.proto                     # Data structures
└── metadata.proto                 # Metadata structures
```

### Enterprise Features (CCL)

```
pkg/ccl/                           # Commercial features
├── changefeedccl/                 # Change data capture
├── backupccl/                     # Backup and restore
├── multitenantccl/                # Multi-tenancy
├── partitionccl/                  # Table partitioning
└── storageccl/                    # Storage extensions
```

### Testing Infrastructure

```
pkg/testutils/                     # Test utilities
├── serverutils/                   # Test server setup
├── testcluster/                   # Multi-node clusters
├── sqlutils/                      # SQL test helpers
├── skip/                          # Conditional skipping
└── lint/                          # Custom linters

pkg/cmd/roachtest/                 # Integration tests
├── tests/                         # Test implementations
├── operations/                    # Test operations
└── roachtestutil/                 # Test utilities

pkg/bench/                         # Benchmarks
├── rttanalysis/                   # Round-trip analysis
└── tpcc/                          # TPC-C benchmarks
```

## Build System

```
build/                             # Build infrastructure
├── bazelutil/                     # Bazel utilities
│   ├── bazel-generate.sh          # Code generation
│   └── distdir_files.bzl          # Distribution files
├── github/                        # GitHub Actions scripts
├── deploy/                        # Deployment configs
│   ├── Dockerfile                 # Main Docker image
│   └── cockroach.sh               # Entrypoint script
├── release/                       # Release scripts
└── toolchains/                    # Cross-compilation

pkg/gen/                           # Code generation
├── gen.bzl                        # Main rules
├── protobuf.bzl                   # Proto generation
├── parser.bzl                     # Parser generation
└── optgen.bzl                     # Optimizer generation
```

## Documentation

```
docs/                              # Documentation
├── RFCS/                          # Design proposals
├── tech-notes/                    # Technical notes
├── design.md                      # Design overview
└── generated/                     # Generated docs

cloud/                             # Deployment docs
├── kubernetes/                    # K8s manifests
│   ├── cockroachdb-statefulset.yaml
│   └── multiregion/               # Multi-region setup
└── docker-compose/                # Compose files
```

## Configuration Files

```
.bazelrc                           # Bazel configuration
.github/workflows/                 # CI/CD workflows
├── github-actions-essential-ci.yml
├── microbenchmarks-ci.yaml
└── code-cover-gen.yml
WORKSPACE                          # Bazel workspace
DEPS.bzl                           # Dependencies
go.mod                             # Go modules
go.sum                             # Dependency checksums
```

## Key Files by Function

### Entry Points
- `pkg/cli/start.go` - Server startup
- `pkg/server/server.go` - Server initialization
- `pkg/sql/conn_executor.go` - SQL execution

### Core Interfaces
- `pkg/kv/kvclient/kvcoord/txn_coord_sender.go` - Transaction coordination
- `pkg/storage/engine.go` - Storage engine interface
- `pkg/sql/execinfra/processorsbase.go` - Execution processor

### Configuration
- `pkg/settings/setting.go` - Cluster settings
- `pkg/cli/flags.go` - CLI flags
- `.bazelrc` - Build configuration

### Testing
- `pkg/testutils/serverutils/api.go` - Test server setup
- `pkg/sql/logictest/logic.go` - Logic test framework
- `pkg/cmd/roachtest/` - Integration tests

---

## Navigation Tips

1. **Finding implementations**: Most interfaces have implementations in the same package
2. **Generated code**: Look for `*_generated.go` or `*.eg.go` files
3. **Test files**: Colocated with source as `*_test.go`
4. **Protocol buffers**: Defined in `*.proto`, generated as `*.pb.go`

## File Naming Conventions

| Pattern | Purpose |
|---------|---------|
| `*_test.go` | Unit tests |
| `*_generated.go` | Generated code |
| `*.eg.go` | Template-generated code |
| `*.proto` | Protocol buffer definitions |
| `doc.go` | Package documentation |
| `BUILD.bazel` | Bazel build rules |

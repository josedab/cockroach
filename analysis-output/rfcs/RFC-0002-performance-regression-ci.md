# RFC-0002: Automated Performance Regression Detection in CI

**Status:** Draft
**Author:** Codebase Analysis
**Created:** November 19, 2025
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Summary

Implement automated performance regression detection in the CI pipeline to catch performance degradations before they reach production.

## Motivation

### Current State

CockroachDB has microbenchmark CI (`.github/workflows/microbenchmarks-ci.yaml`), but:
- Coverage is limited to microbenchmarks
- No automated alerting for regressions
- Results require manual review
- Integration-level performance not tracked

### Problems

1. **Late detection**: Regressions discovered in production
2. **Difficult bisection**: Weeks of commits to review
3. **Silent degradation**: Small regressions accumulate
4. **Manual overhead**: Engineers must review benchmark results

### Proposed Solution

Automated system that:
1. Runs benchmarks on every PR
2. Compares against baseline
3. Blocks merge on significant regression
4. Alerts on gradual degradation

## Detailed Design

### Architecture

```
PR Submitted
    │
    ▼
┌─────────────────┐
│ Trigger Workflow │
└────────┬────────┘
         │
    ┌────┴────┐
    ▼         ▼
┌───────┐ ┌───────┐
│ Micro │ │ Integ │
│ bench │ │ bench │
└───┬───┘ └───┬───┘
    │         │
    └────┬────┘
         │
         ▼
┌─────────────────┐
│ Compare Results │
└────────┬────────┘
         │
    ┌────┴────┐
    ▼         ▼
┌───────┐ ┌───────┐
│ Pass  │ │ Block │
└───────┘ └───────┘
```

### Benchmark Categories

#### 1. Microbenchmarks (Existing)

```bash
./dev test pkg/sql/opt --bench=BenchmarkOptimizer
```

**Metrics**: ns/op, B/op, allocs/op

#### 2. SQL Benchmarks (New)

```sql
-- TPC-C subset
BENCHMARK payment_transaction {
    -- 20 iterations, report p50/p99
}
```

**Metrics**: queries/sec, latency p50/p99

#### 3. KV Benchmarks (New)

```bash
./bin/workload run kv --read-percent=50 --duration=60s
```

**Metrics**: ops/sec, latency p50/p99

### Baseline Management

```yaml
# .github/performance-baselines.yaml
baselines:
  BenchmarkOptimizer:
    ns_op: 1500
    threshold: 10%  # Alert if >10% regression

  tpcc_payment:
    latency_p99_ms: 50
    threshold: 15%
```

**Baseline updates**:
- Automatic when performance improves
- Manual approval for regressions

### Detection Algorithm

```go
func detectRegression(baseline, current Metrics, threshold float64) Decision {
    delta := (current - baseline) / baseline * 100

    if delta > threshold {
        return REGRESSION
    }
    if delta < -threshold {
        // Performance improved
        return IMPROVEMENT
    }
    return STABLE
}
```

### Statistical Significance

To reduce noise:
- Run each benchmark 10 times
- Use Mann-Whitney U test
- Require p < 0.05 for significant difference

```go
func isSignificant(baseline, current []float64) bool {
    _, p := stats.MannWhitneyUTest(baseline, current)
    return p < 0.05
}
```

### GitHub Integration

#### PR Comment

```markdown
## Performance Report

| Benchmark | Baseline | Current | Delta | Status |
|-----------|----------|---------|-------|--------|
| BenchmarkOptimizer | 1500 ns | 1650 ns | +10% | ⚠️ |
| tpcc_payment p99 | 50 ms | 48 ms | -4% | ✅ |

### Details

**BenchmarkOptimizer** shows potential regression:
- Baseline: 1500 ns/op (10 runs, σ=50)
- Current: 1650 ns/op (10 runs, σ=60)
- P-value: 0.003 (significant)

**Suggested actions**:
- Review changes to `pkg/sql/opt/`
- Run with profiling: `go test -bench=. -cpuprofile=cpu.prof`
```

#### Status Check

```
✅ performance/micro: All benchmarks passed
⚠️ performance/sql: 1 regression detected (review required)
```

### Configuration

```yaml
# .github/workflows/performance-ci.yaml
name: Performance CI

on:
  pull_request:
    paths:
      - 'pkg/**'

jobs:
  microbenchmarks:
    runs-on: performance-runner
    steps:
      - name: Run benchmarks
        run: ./build/github/benchmark.sh

      - name: Compare results
        run: ./build/github/compare-benchmarks.sh

      - name: Post results
        uses: actions/github-script@v6
        with:
          script: |
            // Post PR comment with results
```

### Alerting

For gradual degradation (not caught per-PR):

```yaml
# Weekly check
- name: Check trend
  run: |
    ./build/github/check-trend.sh \
      --lookback=30d \
      --threshold=5%

# Alert if 5% degradation over 30 days
```

## Example Usage

### Running Locally

```bash
# Run performance tests
./dev test pkg/sql --bench=. --benchtime=10x

# Compare with baseline
benchstat baseline.txt current.txt
```

### Approving Regression

For intentional regressions (new feature, correctness fix):

```bash
# In PR description
Performance: Approved regression in BenchmarkOptimizer
Reason: Added validation for security fix
Ticket: #12345
```

### Updating Baseline

```bash
# After approved performance improvement
./build/github/update-baseline.sh BenchmarkOptimizer
```

## Implementation Plan

### Phase 1: Infrastructure (3 days)
- [ ] Set up performance runners
- [ ] Create baseline storage
- [ ] Implement comparison script

### Phase 2: Microbenchmarks (2 days)
- [ ] Integrate existing benchmarks
- [ ] Add PR commenting
- [ ] Create status checks

### Phase 3: Integration Benchmarks (5 days)
- [ ] Create SQL benchmark suite
- [ ] Add KV benchmarks
- [ ] Trend analysis

### Phase 4: Polish (2 days)
- [ ] Documentation
- [ ] Onboarding guide
- [ ] Alerting setup

## Backwards Compatibility

- No impact on existing CI
- New checks are additive
- Can be disabled per-PR with label

## Alternatives Considered

### 1. Third-Party Service (Benchstat.io, etc.)

**Pros**: Less maintenance
**Cons**: Data privacy, customization limits
**Decision**: Build in-house for control

### 2. Post-Merge Analysis Only

**Pros**: Faster PR cycle
**Cons**: Late detection, harder bisection
**Decision**: Pre-merge is essential

### 3. Nightly Benchmarks Only

**Pros**: Less CI load
**Cons**: Many commits to bisect
**Decision**: Per-PR is worth the cost

## Open Questions

1. **Hardware consistency**: How to ensure consistent runners?
2. **Noise threshold**: What delta is significant?
3. **Benchmark scope**: Which packages to cover?
4. **History retention**: How long to keep results?

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| Regressions caught pre-merge | 90% | Compare release notes |
| False positive rate | <5% | Manual review |
| Time to detection | <1 day | PR merge to detection |
| Developer satisfaction | Positive | Survey |

## Effort Estimation

- **Total**: 5 dev-days
- **Infrastructure**: 2 days
- **Benchmarks**: 2 days
- **Polish**: 1 day

## Required Approvals

- [ ] CI/Build Team
- [ ] Performance Team
- [ ] Infrastructure (for runners)

## Rollback Strategy

Add `skip-performance-check` label to bypass.

## References

- Existing microbenchmark CI
- Go benchstat documentation
- Performance testing best practices

---

## Appendix: Initial Benchmark Suite

### Microbenchmarks

```
pkg/sql/opt:
  - BenchmarkOptimizer
  - BenchmarkPlanGist

pkg/kv:
  - BenchmarkTxnPut
  - BenchmarkTxnGet

pkg/storage:
  - BenchmarkMVCCPut
  - BenchmarkMVCCScan
```

### Integration Benchmarks

```
tpcc:
  - new_order
  - payment

kv:
  - read-only
  - write-only
  - mixed (50/50)
```

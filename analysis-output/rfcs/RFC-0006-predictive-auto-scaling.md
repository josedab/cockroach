# RFC-0006: Predictive Auto-Scaling Based on Workload Patterns

**Status:** Draft
**Author:** Codebase Analysis
**Created:** November 19, 2025
**Commit SHA:** `6ca473a526d30cccd2d55ea87a6bf4bd22db3e40`

---

## Summary

Implement predictive auto-scaling that anticipates workload changes based on historical patterns, proactively adjusting resources before demand peaks.

## Motivation

### Current State

CockroachDB supports manual scaling and reactive auto-scaling (add nodes when load is high). Problems:

1. **Reactive is too late**: By the time load is high, users experience latency
2. **Manual is error-prone**: Operators must anticipate demand
3. **Over-provisioning**: To avoid #1, operators over-provision

### Proposed Solution

Predictive system that:
1. Learns workload patterns (daily, weekly, seasonal)
2. Predicts future load
3. Proactively scales before demand

## Detailed Design

### Architecture

```
┌────────────────┐
│ Metrics Store  │
│ (Time Series)  │
└───────┬────────┘
        │
        ▼
┌────────────────┐
│ Pattern Learner │
│ (ML Model)     │
└───────┬────────┘
        │
        ▼
┌────────────────┐
│ Load Predictor │
└───────┬────────┘
        │
        ▼
┌────────────────┐     ┌─────────────┐
│ Scaling Engine │────►│ Cluster Ops │
└────────────────┘     └─────────────┘
```

### Metrics Collection

Collect workload indicators:

```go
type WorkloadMetrics struct {
    Timestamp       time.Time
    QueriesPerSec   float64
    ActiveConns     int
    CPUUtilization  float64
    MemoryUsage     float64
    DiskIO          float64
    P99Latency      time.Duration
}
```

**Storage**: Time-series in `system.workload_metrics`.

### Pattern Detection

#### Time-based Patterns

```go
type Pattern struct {
    Type       PatternType  // Daily, Weekly, Seasonal
    PeakTimes  []TimeRange
    ScaleFactor float64
}

// Detect patterns using Fourier analysis
func detectPatterns(metrics []WorkloadMetrics) []Pattern {
    // FFT to find periodic signals
    // Extract dominant frequencies
    // Map to calendar patterns
}
```

#### Event-based Patterns

```go
// Detect non-periodic patterns
type EventPattern struct {
    Trigger     string  // "holiday", "promotion", etc.
    Impact      float64
    LeadTime    time.Duration
}
```

### Prediction Model

Simple approach using historical averages + detected patterns:

```go
func predictLoad(t time.Time, history []WorkloadMetrics, patterns []Pattern) float64 {
    // Base: historical average for this time
    base := historicalAverage(history, t)

    // Apply pattern multipliers
    for _, p := range patterns {
        if p.Applies(t) {
            base *= p.ScaleFactor
        }
    }

    // Add safety margin
    return base * safetyMargin
}
```

Advanced approach (future): LSTM or Prophet model for better accuracy.

### Scaling Decisions

```go
type ScalingDecision struct {
    Action      ScaleAction  // ScaleUp, ScaleDown, NoOp
    TargetNodes int
    Reason      string
    Confidence  float64
}

func decide(predicted, current float64, config ScalingConfig) ScalingDecision {
    ratio := predicted / current

    if ratio > config.ScaleUpThreshold {
        targetNodes := calculateTargetNodes(predicted, config)
        return ScalingDecision{
            Action:      ScaleUp,
            TargetNodes: targetNodes,
            Reason:      fmt.Sprintf("Predicted load %.2f exceeds current capacity", predicted),
            Confidence:  0.85,
        }
    }

    // Similar for scale down
}
```

### Scaling Execution

#### Cloud Environments

```go
// pkg/cloud/autoscale/
type CloudScaler interface {
    ScaleUp(ctx context.Context, count int) error
    ScaleDown(ctx context.Context, nodeIDs []int) error
}

// Implementations for AWS, GCP, Azure
```

#### Kubernetes

```yaml
# CRD for scaling configuration
apiVersion: crdb.cockroachlabs.com/v1
kind: PredictiveScaling
metadata:
  name: my-cluster
spec:
  minNodes: 3
  maxNodes: 10
  patterns:
    - type: daily
      peakHours: [9, 10, 11, 14, 15, 16]
      scaleUpBy: 2
  leadTime: 15m
```

### Configuration

```sql
-- Enable predictive scaling
SET CLUSTER SETTING autoscale.predictive.enabled = true;

-- Configuration
SET CLUSTER SETTING autoscale.min_nodes = 3;
SET CLUSTER SETTING autoscale.max_nodes = 10;
SET CLUSTER SETTING autoscale.lead_time = '15m';
SET CLUSTER SETTING autoscale.confidence_threshold = 0.7;
SET CLUSTER SETTING autoscale.safety_margin = 1.2;
```

### Safety Mechanisms

1. **Confidence threshold**: Only scale if prediction confidence > 70%
2. **Lead time**: Scale up 15 minutes before predicted peak
3. **Cooldown**: Wait 10 minutes between scaling operations
4. **Manual override**: Operators can disable/override
5. **Gradual scale-down**: Remove nodes slowly to avoid disruption

## Example Usage

### Typical Day

```
6:00 AM - Predicted: Low load (night)
          Current: 3 nodes
          Action: None

8:00 AM - Predicted: Peak in 1 hour
          Current: 3 nodes
          Action: Scale up to 6 nodes

9:00 AM - Peak load
          Current: 6 nodes (already scaled)
          Action: None

6:00 PM - Predicted: Declining load
          Current: 6 nodes
          Action: Scale down to 4 nodes

10:00 PM - Predicted: Low load
           Current: 4 nodes
           Action: Scale down to 3 nodes
```

### Monitoring

```sql
-- View predictions
SELECT
    predicted_time,
    predicted_load,
    confidence,
    recommended_action
FROM crdb_internal.scaling_predictions
WHERE predicted_time > now();

-- View scaling history
SELECT
    time,
    action,
    from_nodes,
    to_nodes,
    reason
FROM crdb_internal.scaling_history
ORDER BY time DESC;
```

## Implementation Plan

### Phase 1: Metrics Collection (2 weeks)
- [ ] Define workload metrics
- [ ] Implement collection
- [ ] Create storage schema

### Phase 2: Pattern Detection (3 weeks)
- [ ] Time-series analysis
- [ ] Pattern extraction
- [ ] Validation

### Phase 3: Prediction Engine (2 weeks)
- [ ] Prediction model
- [ ] Confidence scoring
- [ ] Backtesting

### Phase 4: Scaling Integration (3 weeks)
- [ ] Cloud provider integrations
- [ ] Kubernetes operator
- [ ] Safety mechanisms

### Phase 5: Observability (2 weeks)
- [ ] Dashboards
- [ ] Alerts
- [ ] Documentation

## Backwards Compatibility

- Default disabled
- No changes to existing scaling
- Opt-in via cluster setting

## Alternatives Considered

### 1. Reactive Only (Current)

**Pros**: Simple
**Cons**: Too late for users
**Decision**: Predictive adds significant value

### 2. External Service

**Pros**: Specialized ML
**Cons**: Complexity, latency
**Decision**: Build in for tight integration

### 3. Simple Time-Based Rules

**Pros**: Predictable
**Cons**: Can't adapt to changes
**Decision**: Start here, evolve to ML

## Open Questions

1. **Model accuracy**: How accurate can predictions be?
2. **Cost**: Balance resource cost vs performance
3. **Cold start**: How long until useful predictions?
4. **Multi-tenant**: Separate models per tenant?

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| Resource utilization | +15% | Avg CPU/memory |
| P99 latency during peaks | -30% | Latency metrics |
| Scaling lead time | 15+ min | Time before peak |
| Prediction accuracy | >80% | Actual vs predicted |

## Effort Estimation

- **Total**: 60 dev-days (12 weeks)
- **Metrics**: 10 days
- **Patterns**: 15 days
- **Prediction**: 10 days
- **Scaling**: 15 days
- **Observability**: 10 days

## Required Approvals

- [ ] Cloud Team
- [ ] Performance Team
- [ ] Product

## Rollback Strategy

```sql
SET CLUSTER SETTING autoscale.predictive.enabled = false;
```

Falls back to manual/reactive scaling.

## References

- AWS Predictive Scaling
- Google Cloud Autoscaler
- Prophet time series forecasting

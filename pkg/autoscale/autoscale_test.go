// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package autoscale

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWorkloadMetrics(t *testing.T) {
	now := time.Now()
	m := WorkloadMetrics{
		Timestamp:      now,
		QueriesPerSec:  1000,
		ActiveConns:    50,
		CPUUtilization: 75.5,
		MemoryUsage:    60.0,
		DiskIO:         1024000,
		P99Latency:     50 * time.Millisecond,
		NodeID:         1,
	}

	require.Equal(t, now, m.Timestamp)
	require.Equal(t, float64(1000), m.QueriesPerSec)
	require.Equal(t, 50, m.ActiveConns)
	require.Equal(t, 75.5, m.CPUUtilization)
	require.Equal(t, 60.0, m.MemoryUsage)
	require.Equal(t, float64(1024000), m.DiskIO)
	require.Equal(t, 50*time.Millisecond, m.P99Latency)
	require.Equal(t, int32(1), m.NodeID)
}

func TestPatternType(t *testing.T) {
	tests := []struct {
		pt       PatternType
		expected string
	}{
		{PatternDaily, "daily"},
		{PatternWeekly, "weekly"},
		{PatternSeasonal, "seasonal"},
		{PatternCustom, "custom"},
		{PatternType(100), "unknown(100)"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.pt.String())
		})
	}
}

func TestTimeRangeContains(t *testing.T) {
	tests := []struct {
		name     string
		tr       TimeRange
		time     time.Time
		expected bool
	}{
		{
			name:     "within range same day",
			tr:       TimeRange{StartHour: 9, EndHour: 17, DayOfWeek: -1},
			time:     time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "outside range same day",
			tr:       TimeRange{StartHour: 9, EndHour: 17, DayOfWeek: -1},
			time:     time.Date(2024, 1, 15, 20, 0, 0, 0, time.UTC),
			expected: false,
		},
		{
			name:     "correct day of week",
			tr:       TimeRange{StartHour: 9, EndHour: 17, DayOfWeek: 1}, // Monday
			time:     time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),      // Monday
			expected: true,
		},
		{
			name:     "wrong day of week",
			tr:       TimeRange{StartHour: 9, EndHour: 17, DayOfWeek: 2}, // Tuesday
			time:     time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),      // Monday
			expected: false,
		},
		{
			name:     "wrap around midnight - within",
			tr:       TimeRange{StartHour: 22, EndHour: 6, DayOfWeek: -1},
			time:     time.Date(2024, 1, 15, 23, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "wrap around midnight - outside",
			tr:       TimeRange{StartHour: 22, EndHour: 6, DayOfWeek: -1},
			time:     time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.tr.Contains(tc.time)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestPatternApplies(t *testing.T) {
	p := Pattern{
		Type: PatternDaily,
		PeakTimes: []TimeRange{
			{StartHour: 9, EndHour: 11, DayOfWeek: -1},
			{StartHour: 14, EndHour: 16, DayOfWeek: -1},
		},
		ScaleFactor: 1.5,
		Confidence:  0.8,
	}

	// During first peak
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	require.True(t, p.Applies(t1))

	// During second peak
	t2 := time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC)
	require.True(t, p.Applies(t2))

	// Outside peaks
	t3 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	require.False(t, p.Applies(t3))
}

func TestScaleAction(t *testing.T) {
	tests := []struct {
		sa       ScaleAction
		expected string
	}{
		{ScaleNoOp, "no-op"},
		{ScaleUp, "scale-up"},
		{ScaleDown, "scale-down"},
		{ScaleAction(100), "unknown(100)"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.sa.String())
		})
	}
}

func TestDefaultScalingConfig(t *testing.T) {
	config := DefaultScalingConfig()
	require.Equal(t, 3, config.MinNodes)
	require.Equal(t, 10, config.MaxNodes)
	require.Equal(t, 0.8, config.ScaleUpThreshold)
	require.Equal(t, 0.3, config.ScaleDownThreshold)
	require.Equal(t, 15*time.Minute, config.LeadTime)
	require.Equal(t, 10*time.Minute, config.Cooldown)
	require.Equal(t, 0.7, config.ConfidenceThreshold)
	require.Equal(t, 1.2, config.SafetyMargin)
}

func TestPatternDetector(t *testing.T) {
	ctx := context.Background()
	detector := NewPatternDetector()

	// Test with insufficient data
	metrics := []WorkloadMetrics{}
	patterns, err := detector.Detect(ctx, metrics)
	require.NoError(t, err)
	require.Nil(t, patterns)

	// Generate test metrics with daily pattern
	metrics = GenerateTestMetrics(14 * 24 * time.Hour) // 2 weeks
	patterns, err = detector.Detect(ctx, metrics)
	require.NoError(t, err)

	// Should detect at least daily pattern
	found := false
	for _, p := range patterns {
		if p.Type == PatternDaily {
			found = true
			require.Greater(t, p.Confidence, 0.0)
			require.Greater(t, p.ScaleFactor, 1.0)
		}
	}
	require.True(t, found, "Expected to find daily pattern")
}

func TestPredictor(t *testing.T) {
	ctx := context.Background()
	predictor := NewPredictor()

	// Test with no history
	pred, err := predictor.Predict(ctx, time.Now(), nil, nil)
	require.NoError(t, err)
	require.Equal(t, float64(0), pred.Load)
	require.Equal(t, float64(0), pred.Confidence)

	// Generate test metrics
	metrics := GenerateTestMetrics(7 * 24 * time.Hour) // 1 week

	// Predict for next hour
	predTime := time.Now().Add(time.Hour)
	pred, err = predictor.Predict(ctx, predTime, metrics, nil)
	require.NoError(t, err)
	require.Greater(t, pred.Load, 0.0)
	require.Greater(t, pred.Confidence, 0.0)
}

func TestPredictorWithPatterns(t *testing.T) {
	ctx := context.Background()
	predictor := NewPredictor()

	// Generate test metrics
	metrics := GenerateTestMetrics(7 * 24 * time.Hour)

	// Create a pattern that applies
	patterns := []Pattern{
		{
			Type: PatternDaily,
			PeakTimes: []TimeRange{
				{StartHour: 0, EndHour: 24, DayOfWeek: -1}, // All day
			},
			ScaleFactor: 2.0,
			Confidence:  0.9,
		},
	}

	predTime := time.Now().Add(time.Hour)

	// Predict without patterns
	pred1, err := predictor.Predict(ctx, predTime, metrics, nil)
	require.NoError(t, err)

	// Predict with patterns
	pred2, err := predictor.Predict(ctx, predTime, metrics, patterns)
	require.NoError(t, err)

	// Pattern should increase predicted load
	require.Greater(t, pred2.Load, pred1.Load)
}

func TestScalingEngine(t *testing.T) {
	config := DefaultScalingConfig()
	engine := NewScalingEngine(config)

	// Test no-op when current is zero
	decision := engine.Decide(0.5, 0, 3)
	require.Equal(t, ScaleNoOp, decision.Action)

	// Test scale up
	decision = engine.Decide(0.9, 0.5, 3)
	require.Equal(t, ScaleUp, decision.Action)
	require.Greater(t, decision.TargetNodes, 3)

	// Test scale down
	decision = engine.Decide(0.1, 0.5, 6)
	require.Equal(t, ScaleDown, decision.Action)
	require.Less(t, decision.TargetNodes, 6)

	// Test no-op when load is appropriate
	decision = engine.Decide(0.5, 0.5, 5)
	require.Equal(t, ScaleNoOp, decision.Action)
}

func TestScalingEngineConstraints(t *testing.T) {
	config := ScalingConfig{
		MinNodes:           3,
		MaxNodes:           5,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.3,
		SafetyMargin:       1.2,
	}
	engine := NewScalingEngine(config)

	// Test min nodes constraint
	decision := engine.Decide(0.1, 0.5, 5)
	require.GreaterOrEqual(t, decision.TargetNodes, config.MinNodes)

	// Test max nodes constraint
	decision = engine.Decide(5.0, 0.5, 3)
	require.LessOrEqual(t, decision.TargetNodes, config.MaxNodes)
}

func TestInMemoryCollector(t *testing.T) {
	ctx := context.Background()
	collector := NewInMemoryCollector()

	// Test empty collector
	metrics, err := collector.Collect(ctx)
	require.NoError(t, err)
	require.Nil(t, metrics)

	// Add metrics
	now := time.Now()
	m := WorkloadMetrics{
		Timestamp:      now,
		QueriesPerSec:  1000,
		CPUUtilization: 50,
	}
	collector.AddMetric(m)

	// Test collect
	metrics, err = collector.Collect(ctx)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, m.QueriesPerSec, metrics[0].QueriesPerSec)

	// Test count
	require.Equal(t, 1, collector.Count())

	// Test historical
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	metrics, err = collector.GetHistorical(ctx, start, end)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
}

func TestInMemoryCollectorPrune(t *testing.T) {
	collector := NewInMemoryCollector()

	// Add old and new metrics
	oldTime := time.Now().Add(-48 * time.Hour)
	newTime := time.Now()

	collector.AddMetric(WorkloadMetrics{Timestamp: oldTime})
	collector.AddMetric(WorkloadMetrics{Timestamp: newTime})

	require.Equal(t, 2, collector.Count())

	// Prune old metrics
	collector.Prune(24 * time.Hour)

	require.Equal(t, 1, collector.Count())
}

func TestMockScaler(t *testing.T) {
	ctx := context.Background()
	scaler := NewMockScaler(3)

	// Test get current count
	count, err := scaler.GetCurrentNodeCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)

	// Test scale up
	err = scaler.ScaleUp(ctx, 2)
	require.NoError(t, err)
	count, err = scaler.GetCurrentNodeCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 5, count)

	// Test scale down
	err = scaler.ScaleDown(ctx, []int32{4, 5})
	require.NoError(t, err)
	count, err = scaler.GetCurrentNodeCount(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)
}

func TestMockScalerWithCustomFunctions(t *testing.T) {
	ctx := context.Background()
	scaler := NewMockScaler(3)

	// Set custom scale up function that fails
	scaler.SetScaleUpFn(func(count int) error {
		return context.DeadlineExceeded
	})

	err := scaler.ScaleUp(ctx, 1)
	require.Error(t, err)
	require.Equal(t, context.DeadlineExceeded, err)
}

func TestPredictionService(t *testing.T) {
	ctx := context.Background()
	config := DefaultScalingConfig()
	service := NewPredictionService(config)

	// Generate test metrics
	metrics := GenerateTestMetrics(7 * 24 * time.Hour)

	// Generate predictions
	start := time.Now()
	end := start.Add(24 * time.Hour)
	predictions, err := service.GeneratePredictions(ctx, metrics, start, end, time.Hour, 3)
	require.NoError(t, err)
	require.NotEmpty(t, predictions)

	// Verify predictions have required fields
	for _, pred := range predictions {
		require.False(t, pred.Time.IsZero())
		require.GreaterOrEqual(t, pred.Load, 0.0)
		require.GreaterOrEqual(t, pred.Confidence, 0.0)
		require.GreaterOrEqual(t, pred.RecommendedNodes, 0)
	}
}

func TestController(t *testing.T) {
	ctx := context.Background()
	config := DefaultScalingConfig()

	collector := NewInMemoryCollector()
	collector.AddMetrics(GenerateTestMetrics(7 * 24 * time.Hour))

	scaler := NewMockScaler(3)

	controller := NewController(config, scaler, collector)

	// Test get history
	history := controller.GetHistory()
	require.Empty(t, history)

	// Test get predictions
	predictions, err := controller.GetPredictions(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, predictions)
}

func TestGenerateTestMetrics(t *testing.T) {
	duration := 7 * 24 * time.Hour
	metrics := GenerateTestMetrics(duration)

	// Should have approximately 168 hours worth of data
	require.Greater(t, len(metrics), 160)

	// Verify metrics are ordered by time
	for i := 1; i < len(metrics); i++ {
		require.True(t, metrics[i].Timestamp.After(metrics[i-1].Timestamp))
	}

	// Verify all metrics have valid values
	for _, m := range metrics {
		require.GreaterOrEqual(t, m.QueriesPerSec, 0.0)
		require.GreaterOrEqual(t, m.CPUUtilization, 0.0)
		require.LessOrEqual(t, m.CPUUtilization, 100.0)
		require.GreaterOrEqual(t, m.MemoryUsage, 0.0)
		require.LessOrEqual(t, m.MemoryUsage, 100.0)
	}
}

func TestMeanAndStdDev(t *testing.T) {
	// Test mean
	values := []float64{1, 2, 3, 4, 5}
	require.Equal(t, 3.0, mean(values))

	// Empty slice
	require.Equal(t, 0.0, mean(nil))

	// Test standard deviation
	stdDev := standardDeviation(values)
	require.InDelta(t, 1.58, stdDev, 0.01)

	// Single value
	require.Equal(t, 0.0, standardDeviation([]float64{5}))
}

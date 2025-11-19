// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package sql

import (
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/sql/appstatspb"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func TestCalculateAdaptiveTimeout(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	testCases := []struct {
		name           string
		stats          QueryLatencyStats
		config         AdaptiveTimeoutConfig
		expectedResult time.Duration
	}{
		{
			name: "not enough samples uses default",
			stats: QueryLatencyStats{
				ExecutionCount: 5,
				P99Latency:     100 * time.Millisecond,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     2.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: 30 * time.Second,
		},
		{
			name: "sufficient samples uses P99 * multiplier",
			stats: QueryLatencyStats{
				ExecutionCount: 100,
				P99Latency:     100 * time.Millisecond,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     2.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: 200 * time.Millisecond, // But clamped to min
		},
		{
			name: "result clamped to minimum",
			stats: QueryLatencyStats{
				ExecutionCount: 100,
				P99Latency:     100 * time.Millisecond,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     2.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: time.Second, // Clamped to min
		},
		{
			name: "result clamped to maximum",
			stats: QueryLatencyStats{
				ExecutionCount: 100,
				P99Latency:     10 * time.Minute,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     2.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: 5 * time.Minute, // Clamped to max
		},
		{
			name: "within bounds",
			stats: QueryLatencyStats{
				ExecutionCount: 100,
				P99Latency:     time.Second,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     2.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: 2 * time.Second, // P99 * multiplier
		},
		{
			name: "different multiplier",
			stats: QueryLatencyStats{
				ExecutionCount: 50,
				P99Latency:     5 * time.Second,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     3.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: 15 * time.Second, // 5s * 3.0
		},
		{
			name: "exactly at min samples",
			stats: QueryLatencyStats{
				ExecutionCount: 10,
				P99Latency:     2 * time.Second,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     2.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: 4 * time.Second, // 2s * 2.0
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CalculateAdaptiveTimeout(tc.stats, tc.config)
			require.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestStatsToQueryLatencyStats(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	stats := &appstatspb.StatementStatistics{
		Count: 100,
		LatencyInfo: appstatspb.LatencyInfo{
			P50: 0.050, // 50ms in seconds
			P99: 0.200, // 200ms in seconds
			Max: 1.000, // 1s in seconds
		},
		ServiceLat: appstatspb.NumericStat{
			Mean: 0.075, // 75ms in seconds
		},
	}

	latencyStats := StatsToQueryLatencyStats(stats)

	require.Equal(t, int64(100), latencyStats.ExecutionCount)
	require.Equal(t, 50*time.Millisecond, latencyStats.P50Latency)
	require.Equal(t, 200*time.Millisecond, latencyStats.P99Latency)
	require.Equal(t, time.Second, latencyStats.MaxLatency)
	require.Equal(t, 75*time.Millisecond, latencyStats.MeanLatency)
}

func TestAdaptiveTimeoutConfigDefaults(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	// Test with no stats and not enough samples
	stats := QueryLatencyStats{
		ExecutionCount: 0,
		P99Latency:     0,
	}

	config := AdaptiveTimeoutConfig{
		MinSamples:     10,
		Multiplier:     2.0,
		MinTimeout:     time.Second,
		MaxTimeout:     5 * time.Minute,
		DefaultTimeout: 30 * time.Second,
	}

	result := CalculateAdaptiveTimeout(stats, config)
	require.Equal(t, 30*time.Second, result, "should use default when no samples")
}

func TestCalculateAdaptiveTimeoutEdgeCases(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	testCases := []struct {
		name           string
		stats          QueryLatencyStats
		config         AdaptiveTimeoutConfig
		expectedResult time.Duration
	}{
		{
			name: "zero P99 latency",
			stats: QueryLatencyStats{
				ExecutionCount: 100,
				P99Latency:     0,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     2.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: time.Second, // Clamped to min
		},
		{
			name: "very high multiplier",
			stats: QueryLatencyStats{
				ExecutionCount: 100,
				P99Latency:     time.Minute,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     10.0,
				MinTimeout:     time.Second,
				MaxTimeout:     5 * time.Minute,
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: 5 * time.Minute, // Clamped to max
		},
		{
			name: "min equals max",
			stats: QueryLatencyStats{
				ExecutionCount: 100,
				P99Latency:     time.Second,
			},
			config: AdaptiveTimeoutConfig{
				MinSamples:     10,
				Multiplier:     2.0,
				MinTimeout:     30 * time.Second,
				MaxTimeout:     30 * time.Second, // Same as min
				DefaultTimeout: 30 * time.Second,
			},
			expectedResult: 30 * time.Second, // Clamped to min (which equals max)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CalculateAdaptiveTimeout(tc.stats, tc.config)
			require.Equal(t, tc.expectedResult, result)
		})
	}
}

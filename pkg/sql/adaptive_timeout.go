// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package sql

import (
	"context"
	"time"

	"github.com/cockroachdb/cockroach/pkg/settings"
	"github.com/cockroachdb/cockroach/pkg/sql/appstatspb"
	"github.com/cockroachdb/cockroach/pkg/util/log"
)

// Cluster settings for adaptive timeout configuration.
var (
	// AdaptiveTimeoutEnabled controls whether adaptive timeouts are enabled.
	AdaptiveTimeoutEnabled = settings.RegisterBoolSetting(
		settings.ApplicationLevel,
		"sql.adaptive_timeout.enabled",
		"if true, enables adaptive query timeouts based on historical execution patterns",
		false,
		settings.WithPublic)

	// AdaptiveTimeoutMultiplier is the multiplier applied to the P99 latency
	// to calculate the adaptive timeout.
	AdaptiveTimeoutMultiplier = settings.RegisterFloatSetting(
		settings.ApplicationLevel,
		"sql.adaptive_timeout.multiplier",
		"multiplier applied to P99 latency for adaptive timeout calculation",
		2.0,
		settings.FloatInRange(1.0, 10.0),
		settings.WithPublic)

	// AdaptiveTimeoutMinSamples is the minimum number of samples required
	// before using adaptive timeout.
	AdaptiveTimeoutMinSamples = settings.RegisterIntSetting(
		settings.ApplicationLevel,
		"sql.adaptive_timeout.min_samples",
		"minimum number of executions before using adaptive timeout for a query",
		10,
		settings.PositiveInt,
		settings.WithPublic)

	// AdaptiveTimeoutMin is the minimum adaptive timeout value.
	AdaptiveTimeoutMin = settings.RegisterDurationSetting(
		settings.ApplicationLevel,
		"sql.adaptive_timeout.min",
		"minimum value for adaptive timeout",
		time.Second,
		settings.PositiveDuration,
		settings.WithPublic)

	// AdaptiveTimeoutMax is the maximum adaptive timeout value.
	AdaptiveTimeoutMax = settings.RegisterDurationSetting(
		settings.ApplicationLevel,
		"sql.adaptive_timeout.max",
		"maximum value for adaptive timeout",
		5*time.Minute,
		settings.PositiveDuration,
		settings.WithPublic)

	// AdaptiveTimeoutDefault is the default timeout when no history exists.
	AdaptiveTimeoutDefault = settings.RegisterDurationSetting(
		settings.ApplicationLevel,
		"sql.adaptive_timeout.default",
		"default timeout when no historical execution data exists",
		30*time.Second,
		settings.NonNegativeDuration,
		settings.WithPublic)
)

// AdaptiveTimeoutConfig holds the configuration for adaptive timeout calculation.
type AdaptiveTimeoutConfig struct {
	Enabled        bool
	Multiplier     float64
	MinSamples     int64
	MinTimeout     time.Duration
	MaxTimeout     time.Duration
	DefaultTimeout time.Duration
}

// GetAdaptiveTimeoutConfig retrieves the current adaptive timeout configuration
// from cluster settings.
func GetAdaptiveTimeoutConfig(sv *settings.Values) AdaptiveTimeoutConfig {
	return AdaptiveTimeoutConfig{
		Enabled:        AdaptiveTimeoutEnabled.Get(sv),
		Multiplier:     AdaptiveTimeoutMultiplier.Get(sv),
		MinSamples:     AdaptiveTimeoutMinSamples.Get(sv),
		MinTimeout:     AdaptiveTimeoutMin.Get(sv),
		MaxTimeout:     AdaptiveTimeoutMax.Get(sv),
		DefaultTimeout: AdaptiveTimeoutDefault.Get(sv),
	}
}

// QueryLatencyStats holds the latency statistics for a query fingerprint.
type QueryLatencyStats struct {
	ExecutionCount int64
	P50Latency     time.Duration
	P99Latency     time.Duration
	MaxLatency     time.Duration
	MeanLatency    time.Duration
}

// CalculateAdaptiveTimeout calculates the adaptive timeout based on historical
// execution statistics.
func CalculateAdaptiveTimeout(
	stats QueryLatencyStats, config AdaptiveTimeoutConfig,
) time.Duration {
	// If we don't have enough samples, use the default timeout
	if stats.ExecutionCount < config.MinSamples {
		return config.DefaultTimeout
	}

	// Calculate adaptive timeout as P99 * multiplier
	adaptive := time.Duration(float64(stats.P99Latency) * config.Multiplier)

	// Clamp to configured bounds
	if adaptive < config.MinTimeout {
		return config.MinTimeout
	}
	if adaptive > config.MaxTimeout {
		return config.MaxTimeout
	}

	return adaptive
}

// StatsToQueryLatencyStats converts statement statistics to QueryLatencyStats.
func StatsToQueryLatencyStats(stats *appstatspb.StatementStatistics) QueryLatencyStats {
	return QueryLatencyStats{
		ExecutionCount: stats.Count,
		P50Latency:     time.Duration(stats.LatencyInfo.P50 * float64(time.Second)),
		P99Latency:     time.Duration(stats.LatencyInfo.P99 * float64(time.Second)),
		MaxLatency:     time.Duration(stats.LatencyInfo.Max * float64(time.Second)),
		MeanLatency:    time.Duration(stats.ServiceLat.Mean * float64(time.Second)),
	}
}

// AdaptiveTimeoutCalculator provides methods for calculating adaptive timeouts
// during query execution.
type AdaptiveTimeoutCalculator struct {
	sv     *settings.Values
	config AdaptiveTimeoutConfig
}

// NewAdaptiveTimeoutCalculator creates a new AdaptiveTimeoutCalculator.
func NewAdaptiveTimeoutCalculator(sv *settings.Values) *AdaptiveTimeoutCalculator {
	return &AdaptiveTimeoutCalculator{
		sv:     sv,
		config: GetAdaptiveTimeoutConfig(sv),
	}
}

// RefreshConfig refreshes the configuration from cluster settings.
func (c *AdaptiveTimeoutCalculator) RefreshConfig() {
	c.config = GetAdaptiveTimeoutConfig(c.sv)
}

// IsEnabled returns whether adaptive timeout is enabled.
func (c *AdaptiveTimeoutCalculator) IsEnabled() bool {
	return c.config.Enabled
}

// GetTimeout calculates the timeout for a query based on its statistics.
// If adaptive timeout is disabled or not applicable, returns 0.
func (c *AdaptiveTimeoutCalculator) GetTimeout(
	ctx context.Context,
	fingerprintID appstatspb.StmtFingerprintID,
	getStats func(appstatspb.StmtFingerprintID) (*appstatspb.StatementStatistics, bool),
) time.Duration {
	if !c.config.Enabled {
		return 0
	}

	stats, found := getStats(fingerprintID)
	if !found {
		if log.V(2) {
			log.Infof(ctx, "adaptive timeout: no stats for fingerprint %d, using default %s",
				fingerprintID, c.config.DefaultTimeout)
		}
		return c.config.DefaultTimeout
	}

	latencyStats := StatsToQueryLatencyStats(stats)
	timeout := CalculateAdaptiveTimeout(latencyStats, c.config)

	if log.V(2) {
		log.Infof(ctx, "adaptive timeout: fingerprint %d, count=%d, p99=%s, timeout=%s",
			fingerprintID, latencyStats.ExecutionCount, latencyStats.P99Latency, timeout)
	}

	return timeout
}

// GetDefaultTimeout returns the default timeout when no statistics exist.
func (c *AdaptiveTimeoutCalculator) GetDefaultTimeout() time.Duration {
	return c.config.DefaultTimeout
}

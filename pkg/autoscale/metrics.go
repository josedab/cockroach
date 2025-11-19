// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package autoscale

import (
	"context"
	"sort"
	"sync"
	"time"
)

// InMemoryCollector is an in-memory implementation of MetricsCollector for testing.
// In production, this would be replaced with a collector that reads from
// system.workload_metrics.
type InMemoryCollector struct {
	mu      sync.RWMutex
	metrics []WorkloadMetrics
}

// NewInMemoryCollector creates a new InMemoryCollector.
func NewInMemoryCollector() *InMemoryCollector {
	return &InMemoryCollector{
		metrics: make([]WorkloadMetrics, 0),
	}
}

// Collect gathers current workload metrics.
// In production, this would query the actual CockroachDB metrics.
func (c *InMemoryCollector) Collect(ctx context.Context) ([]WorkloadMetrics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy of the most recent metrics
	if len(c.metrics) == 0 {
		return nil, nil
	}

	result := make([]WorkloadMetrics, 1)
	result[0] = c.metrics[len(c.metrics)-1]
	return result, nil
}

// GetHistorical returns historical metrics for the given time range.
func (c *InMemoryCollector) GetHistorical(
	ctx context.Context, start, end time.Time,
) ([]WorkloadMetrics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []WorkloadMetrics
	for _, m := range c.metrics {
		if (m.Timestamp.Equal(start) || m.Timestamp.After(start)) &&
			(m.Timestamp.Equal(end) || m.Timestamp.Before(end)) {
			result = append(result, m)
		}
	}

	// Sort by timestamp
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	return result, nil
}

// AddMetric adds a metric to the collector.
func (c *InMemoryCollector) AddMetric(m WorkloadMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = append(c.metrics, m)
}

// AddMetrics adds multiple metrics to the collector.
func (c *InMemoryCollector) AddMetrics(metrics []WorkloadMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = append(c.metrics, metrics...)
}

// Clear removes all metrics from the collector.
func (c *InMemoryCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = make([]WorkloadMetrics, 0)
}

// Count returns the number of metrics in the collector.
func (c *InMemoryCollector) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.metrics)
}

// Prune removes metrics older than the retention period.
func (c *InMemoryCollector) Prune(retention time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-retention)
	var kept []WorkloadMetrics
	for _, m := range c.metrics {
		if m.Timestamp.After(cutoff) {
			kept = append(kept, m)
		}
	}
	c.metrics = kept
}

// GenerateTestMetrics creates synthetic metrics for testing.
// It generates hourly data with daily patterns for the specified duration.
func GenerateTestMetrics(duration time.Duration) []WorkloadMetrics {
	var metrics []WorkloadMetrics

	end := time.Now()
	start := end.Add(-duration)
	interval := time.Hour

	for t := start; t.Before(end); t = t.Add(interval) {
		// Create a daily pattern: high during business hours, low at night
		hour := t.Hour()
		var baseLoad float64

		switch {
		case hour >= 9 && hour <= 11:
			// Morning peak
			baseLoad = 0.8
		case hour >= 14 && hour <= 16:
			// Afternoon peak
			baseLoad = 0.7
		case hour >= 6 && hour <= 20:
			// Normal business hours
			baseLoad = 0.4
		default:
			// Night
			baseLoad = 0.1
		}

		// Add some variation for day of week
		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			baseLoad *= 0.5 // Weekend is quieter
		}

		// Add some random variation (±10%)
		variation := (float64((t.Unix()%100)-50) / 500.0)
		load := baseLoad + variation
		if load < 0 {
			load = 0
		}
		if load > 1 {
			load = 1
		}

		metrics = append(metrics, WorkloadMetrics{
			Timestamp:      t,
			QueriesPerSec:  load * 10000,
			ActiveConns:    int(load * 500),
			CPUUtilization: load * 100,
			MemoryUsage:    load * 80,
			DiskIO:         load * 1000000,
			P99Latency:     time.Duration(10+load*100) * time.Millisecond,
			NodeID:         1,
		})
	}

	return metrics
}

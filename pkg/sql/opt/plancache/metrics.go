// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package plancache

import "github.com/cockroachdb/cockroach/pkg/util/metric"

// Metrics contains metrics for the query plan cache.
type Metrics struct {
	// Hits is the number of cache hits.
	Hits *metric.Counter

	// Misses is the number of cache misses.
	Misses *metric.Counter

	// Evictions is the number of entries evicted from the cache.
	Evictions *metric.Counter

	// Size is the current number of entries in the cache.
	Size *metric.Gauge

	// MemoryBytes is the estimated memory usage of the cache.
	MemoryBytes *metric.Gauge
}

// MetricStruct implements the metric.Struct interface.
func (Metrics) MetricStruct() {}

var (
	metaHits = metric.Metadata{
		Name:        "sql.opt.plan_cache.hits",
		Help:        "Number of query plan cache hits",
		Measurement: "Cache Hits",
		Unit:        metric.Unit_COUNT,
	}
	metaMisses = metric.Metadata{
		Name:        "sql.opt.plan_cache.misses",
		Help:        "Number of query plan cache misses",
		Measurement: "Cache Misses",
		Unit:        metric.Unit_COUNT,
	}
	metaEvictions = metric.Metadata{
		Name:        "sql.opt.plan_cache.evictions",
		Help:        "Number of plans evicted from cache",
		Measurement: "Evictions",
		Unit:        metric.Unit_COUNT,
	}
	metaSize = metric.Metadata{
		Name:        "sql.opt.plan_cache.size",
		Help:        "Number of plans currently in cache",
		Measurement: "Plans",
		Unit:        metric.Unit_COUNT,
	}
	metaMemoryBytes = metric.Metadata{
		Name:        "sql.opt.plan_cache.memory_bytes",
		Help:        "Estimated memory used by the plan cache",
		Measurement: "Bytes",
		Unit:        metric.Unit_BYTES,
	}
)

// NewMetrics creates a new Metrics instance.
func NewMetrics() *Metrics {
	return &Metrics{
		Hits:        metric.NewCounter(metaHits),
		Misses:      metric.NewCounter(metaMisses),
		Evictions:   metric.NewCounter(metaEvictions),
		Size:        metric.NewGauge(metaSize),
		MemoryBytes: metric.NewGauge(metaMemoryBytes),
	}
}

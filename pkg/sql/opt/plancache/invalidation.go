// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package plancache

import (
	"time"

	"github.com/cockroachdb/cockroach/pkg/sql/catalog/descpb"
)

// InvalidationReason describes why a cached plan was invalidated.
type InvalidationReason int

const (
	// InvalidationNone indicates no invalidation occurred.
	InvalidationNone InvalidationReason = iota

	// InvalidationSchemaChange indicates the plan was invalidated due to
	// a schema change (table/index modified, dropped, etc.).
	InvalidationSchemaChange

	// InvalidationStatsUpdate indicates the plan was invalidated due to
	// table statistics being updated.
	InvalidationStatsUpdate

	// InvalidationTTLExpired indicates the plan was invalidated because
	// it exceeded the configured TTL.
	InvalidationTTLExpired

	// InvalidationManual indicates the plan was manually invalidated
	// (e.g., via DISCARD PLANS).
	InvalidationManual

	// InvalidationMemoryPressure indicates the plan was evicted due to
	// memory pressure.
	InvalidationMemoryPressure
)

// String returns a human-readable name for the invalidation reason.
func (r InvalidationReason) String() string {
	switch r {
	case InvalidationNone:
		return "none"
	case InvalidationSchemaChange:
		return "schema_change"
	case InvalidationStatsUpdate:
		return "stats_update"
	case InvalidationTTLExpired:
		return "ttl_expired"
	case InvalidationManual:
		return "manual"
	case InvalidationMemoryPressure:
		return "memory_pressure"
	default:
		return "unknown"
	}
}

// InvalidationEvent represents a cache invalidation event.
type InvalidationEvent struct {
	// Reason is why the invalidation occurred.
	Reason InvalidationReason

	// TableID is the table that triggered the invalidation (if applicable).
	TableID descpb.ID

	// Timestamp is when the invalidation occurred.
	Timestamp time.Time

	// EntriesInvalidated is the count of cache entries that were invalidated.
	EntriesInvalidated int
}

// Invalidator manages cache invalidation logic.
type Invalidator struct {
	sessionCache *SessionPlanCache
	clusterCache *ClusterPlanCache
	ttl          time.Duration
}

// NewInvalidator creates a new cache invalidator.
func NewInvalidator(
	sessionCache *SessionPlanCache, clusterCache *ClusterPlanCache, ttl time.Duration,
) *Invalidator {
	return &Invalidator{
		sessionCache: sessionCache,
		clusterCache: clusterCache,
		ttl:          ttl,
	}
}

// InvalidateForSchemaChange invalidates all plans that depend on the given table.
func (inv *Invalidator) InvalidateForSchemaChange(tableID descpb.ID) InvalidationEvent {
	event := InvalidationEvent{
		Reason:    InvalidationSchemaChange,
		TableID:   tableID,
		Timestamp: time.Now(),
	}

	// Count entries before invalidation.
	sessionBefore := 0
	clusterBefore := 0
	if inv.sessionCache != nil {
		sessionBefore = inv.sessionCache.Len()
	}
	if inv.clusterCache != nil {
		clusterBefore = inv.clusterCache.Len()
	}

	// Invalidate session cache.
	if inv.sessionCache != nil {
		inv.sessionCache.InvalidateTable(tableID)
	}

	// Invalidate cluster cache.
	if inv.clusterCache != nil {
		inv.clusterCache.InvalidateTable(tableID)
	}

	// Calculate entries invalidated.
	sessionAfter := 0
	clusterAfter := 0
	if inv.sessionCache != nil {
		sessionAfter = inv.sessionCache.Len()
	}
	if inv.clusterCache != nil {
		clusterAfter = inv.clusterCache.Len()
	}
	event.EntriesInvalidated = (sessionBefore - sessionAfter) + (clusterBefore - clusterAfter)

	return event
}

// InvalidateForStatsUpdate invalidates plans for tables whose statistics were updated.
// This ensures the optimizer can use the new statistics.
func (inv *Invalidator) InvalidateForStatsUpdate(tableID descpb.ID) InvalidationEvent {
	event := InvalidationEvent{
		Reason:    InvalidationStatsUpdate,
		TableID:   tableID,
		Timestamp: time.Now(),
	}

	// Count entries before invalidation.
	sessionBefore := 0
	clusterBefore := 0
	if inv.sessionCache != nil {
		sessionBefore = inv.sessionCache.Len()
	}
	if inv.clusterCache != nil {
		clusterBefore = inv.clusterCache.Len()
	}

	// Invalidate session cache.
	if inv.sessionCache != nil {
		inv.sessionCache.InvalidateTable(tableID)
	}

	// Invalidate cluster cache.
	if inv.clusterCache != nil {
		inv.clusterCache.InvalidateTable(tableID)
	}

	// Calculate entries invalidated.
	sessionAfter := 0
	clusterAfter := 0
	if inv.sessionCache != nil {
		sessionAfter = inv.sessionCache.Len()
	}
	if inv.clusterCache != nil {
		clusterAfter = inv.clusterCache.Len()
	}
	event.EntriesInvalidated = (sessionBefore - sessionAfter) + (clusterBefore - clusterAfter)

	return event
}

// InvalidateExpired invalidates all plans that have exceeded the TTL.
func (inv *Invalidator) InvalidateExpired() InvalidationEvent {
	event := InvalidationEvent{
		Reason:    InvalidationTTLExpired,
		Timestamp: time.Now(),
	}

	if inv.ttl == 0 {
		// TTL-based invalidation is disabled.
		return event
	}

	cutoff := time.Now().Add(-inv.ttl)

	// Invalidate expired entries in session cache.
	if inv.sessionCache != nil {
		for hash, entry := range inv.sessionCache.entries {
			if entry.ValidSince.Before(cutoff) {
				inv.sessionCache.lru.Remove(entry.lruElement)
				delete(inv.sessionCache.entries, hash)
				event.EntriesInvalidated++
			}
		}
		if inv.sessionCache.metrics != nil {
			inv.sessionCache.metrics.Size.Update(int64(len(inv.sessionCache.entries)))
		}
	}

	// Invalidate expired entries in cluster cache.
	if inv.clusterCache != nil {
		inv.clusterCache.mu.Lock()
		for hash, entry := range inv.clusterCache.mu.entries {
			if entry.ValidSince.Before(cutoff) {
				inv.clusterCache.mu.lru.Remove(entry.lruElement)
				delete(inv.clusterCache.mu.entries, hash)
				event.EntriesInvalidated++
			}
		}
		if inv.clusterCache.metrics != nil {
			inv.clusterCache.metrics.Size.Update(int64(len(inv.clusterCache.mu.entries)))
		}
		inv.clusterCache.mu.Unlock()
	}

	return event
}

// ClearAll clears all cached plans from both session and cluster caches.
func (inv *Invalidator) ClearAll() InvalidationEvent {
	event := InvalidationEvent{
		Reason:    InvalidationManual,
		Timestamp: time.Now(),
	}

	// Count entries before clearing.
	if inv.sessionCache != nil {
		event.EntriesInvalidated += inv.sessionCache.Len()
		inv.sessionCache.Clear()
	}

	if inv.clusterCache != nil {
		event.EntriesInvalidated += inv.clusterCache.Len()
		inv.clusterCache.Clear()
	}

	return event
}

// IsValid checks if a cache entry is still valid.
// It checks the TTL and can be extended with other validity checks.
func (inv *Invalidator) IsValid(entry *CacheEntry) bool {
	if entry == nil {
		return false
	}

	// Check TTL.
	if inv.ttl > 0 && time.Since(entry.ValidSince) > inv.ttl {
		return false
	}

	return true
}

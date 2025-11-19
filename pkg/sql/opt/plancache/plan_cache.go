// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// Package plancache implements a query plan cache for the SQL optimizer.
// It stores and reuses optimized query plans for repeated queries, reducing
// optimization overhead for common query patterns.
package plancache

import (
	"container/list"
	"sync"
	"time"

	"github.com/cockroachdb/cockroach/pkg/sql/catalog/descpb"
	"github.com/cockroachdb/cockroach/pkg/sql/opt/memo"
	"github.com/cockroachdb/cockroach/pkg/sql/opt/props/physical"
	"github.com/cockroachdb/cockroach/pkg/util/syncutil"
)

// CacheKey represents the key used to look up cached query plans.
// It includes the query fingerprint and relevant settings that affect optimization.
type CacheKey struct {
	// Fingerprint is the normalized query string with placeholders.
	Fingerprint string

	// Database is the current database context.
	Database string

	// SearchPath is the schema search path.
	SearchPath string

	// DistSQLMode affects how queries are planned for distribution.
	DistSQLMode string

	// VectorizeMode affects vectorized execution planning.
	VectorizeMode string
}

// Hash returns a string hash of the cache key for map lookups.
func (k CacheKey) Hash() string {
	// Simple concatenation with delimiters for hashing.
	// Using null bytes as delimiters to avoid collisions.
	return k.Fingerprint + "\x00" + k.Database + "\x00" + k.SearchPath + "\x00" + k.DistSQLMode + "\x00" + k.VectorizeMode
}

// PlanStats tracks usage statistics for a cached plan.
type PlanStats struct {
	// HitCount is the number of times this plan was retrieved from cache.
	HitCount int64

	// TotalLatency is the cumulative execution time for this plan.
	TotalLatency time.Duration

	// LastUsed is the timestamp of the last cache hit.
	LastUsed time.Time
}

// CacheEntry represents a cached query plan.
type CacheEntry struct {
	// Key is the cache key for this entry.
	Key CacheKey

	// Memo is the optimized memo structure.
	Memo *memo.Memo

	// PhysProps are the required physical properties for the plan.
	PhysProps *physical.Required

	// Stats tracks usage statistics for this entry.
	Stats PlanStats

	// ValidSince is when this entry was added to the cache.
	ValidSince time.Time

	// Dependencies are the table IDs that this plan depends on.
	// Used for invalidation when schemas change.
	Dependencies []descpb.ID

	// lruElement is the element in the LRU list for eviction tracking.
	lruElement *list.Element
}

// SessionPlanCache is a per-session plan cache.
// It is not thread-safe as it's only accessed from a single session.
type SessionPlanCache struct {
	entries map[string]*CacheEntry
	lru     *list.List
	maxSize int
	metrics *Metrics
}

// NewSessionPlanCache creates a new session-level plan cache.
func NewSessionPlanCache(maxSize int, metrics *Metrics) *SessionPlanCache {
	return &SessionPlanCache{
		entries: make(map[string]*CacheEntry),
		lru:     list.New(),
		maxSize: maxSize,
		metrics: metrics,
	}
}

// Lookup retrieves a cached plan for the given key.
// Returns the entry and true if found, nil and false otherwise.
func (c *SessionPlanCache) Lookup(key CacheKey) (*CacheEntry, bool) {
	hash := key.Hash()
	entry, ok := c.entries[hash]
	if !ok {
		if c.metrics != nil {
			c.metrics.Misses.Inc(1)
		}
		return nil, false
	}

	// Update LRU position.
	c.lru.MoveToFront(entry.lruElement)

	// Update stats.
	entry.Stats.HitCount++
	entry.Stats.LastUsed = time.Now()

	if c.metrics != nil {
		c.metrics.Hits.Inc(1)
	}

	return entry, true
}

// Insert adds a new plan to the cache.
func (c *SessionPlanCache) Insert(
	key CacheKey, m *memo.Memo, physProps *physical.Required, deps []descpb.ID,
) {
	hash := key.Hash()

	// Check if entry already exists.
	if existing, ok := c.entries[hash]; ok {
		// Update existing entry.
		existing.Memo = m
		existing.PhysProps = physProps
		existing.Dependencies = deps
		existing.ValidSince = time.Now()
		c.lru.MoveToFront(existing.lruElement)
		return
	}

	// Evict if at capacity.
	for len(c.entries) >= c.maxSize {
		c.evictLRU()
	}

	// Create new entry.
	entry := &CacheEntry{
		Key:          key,
		Memo:         m,
		PhysProps:    physProps,
		ValidSince:   time.Now(),
		Dependencies: deps,
		Stats: PlanStats{
			LastUsed: time.Now(),
		},
	}
	entry.lruElement = c.lru.PushFront(entry)
	c.entries[hash] = entry

	if c.metrics != nil {
		c.metrics.Size.Update(int64(len(c.entries)))
	}
}

// evictLRU removes the least recently used entry.
func (c *SessionPlanCache) evictLRU() {
	if c.lru.Len() == 0 {
		return
	}

	// Get the back element (LRU).
	elem := c.lru.Back()
	if elem == nil {
		return
	}

	entry := elem.Value.(*CacheEntry)
	c.lru.Remove(elem)
	delete(c.entries, entry.Key.Hash())

	if c.metrics != nil {
		c.metrics.Evictions.Inc(1)
		c.metrics.Size.Update(int64(len(c.entries)))
	}
}

// InvalidateTable removes all entries that depend on the given table.
func (c *SessionPlanCache) InvalidateTable(tableID descpb.ID) {
	for hash, entry := range c.entries {
		for _, dep := range entry.Dependencies {
			if dep == tableID {
				c.lru.Remove(entry.lruElement)
				delete(c.entries, hash)
				if c.metrics != nil {
					c.metrics.Evictions.Inc(1)
				}
				break
			}
		}
	}
	if c.metrics != nil {
		c.metrics.Size.Update(int64(len(c.entries)))
	}
}

// Clear removes all entries from the cache.
func (c *SessionPlanCache) Clear() {
	c.entries = make(map[string]*CacheEntry)
	c.lru.Init()
	if c.metrics != nil {
		c.metrics.Size.Update(0)
	}
}

// Len returns the number of entries in the cache.
func (c *SessionPlanCache) Len() int {
	return len(c.entries)
}

// GetEntries returns a copy of all cache entries for inspection.
func (c *SessionPlanCache) GetEntries() []*CacheEntry {
	entries := make([]*CacheEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, entry)
	}
	return entries
}

// ClusterPlanCache is a shared plan cache across all sessions.
// It is thread-safe and uses sharding to reduce lock contention.
type ClusterPlanCache struct {
	mu struct {
		syncutil.RWMutex
		entries map[string]*CacheEntry
		lru     *list.List
	}
	maxSize int
	metrics *Metrics
}

// NewClusterPlanCache creates a new cluster-level plan cache.
func NewClusterPlanCache(maxSize int, metrics *Metrics) *ClusterPlanCache {
	c := &ClusterPlanCache{
		maxSize: maxSize,
		metrics: metrics,
	}
	c.mu.entries = make(map[string]*CacheEntry)
	c.mu.lru = list.New()
	return c
}

// Lookup retrieves a cached plan for the given key.
// Returns the entry and true if found, nil and false otherwise.
func (c *ClusterPlanCache) Lookup(key CacheKey) (*CacheEntry, bool) {
	hash := key.Hash()

	c.mu.RLock()
	entry, ok := c.mu.entries[hash]
	c.mu.RUnlock()

	if !ok {
		if c.metrics != nil {
			c.metrics.Misses.Inc(1)
		}
		return nil, false
	}

	// Update LRU and stats (requires write lock).
	c.mu.Lock()
	// Re-check after acquiring write lock.
	entry, ok = c.mu.entries[hash]
	if ok {
		c.mu.lru.MoveToFront(entry.lruElement)
		entry.Stats.HitCount++
		entry.Stats.LastUsed = time.Now()
	}
	c.mu.Unlock()

	if c.metrics != nil {
		if ok {
			c.metrics.Hits.Inc(1)
		} else {
			c.metrics.Misses.Inc(1)
		}
	}

	return entry, ok
}

// Insert adds a new plan to the cache.
func (c *ClusterPlanCache) Insert(
	key CacheKey, m *memo.Memo, physProps *physical.Required, deps []descpb.ID,
) {
	hash := key.Hash()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if entry already exists.
	if existing, ok := c.mu.entries[hash]; ok {
		// Update existing entry.
		existing.Memo = m
		existing.PhysProps = physProps
		existing.Dependencies = deps
		existing.ValidSince = time.Now()
		c.mu.lru.MoveToFront(existing.lruElement)
		return
	}

	// Evict if at capacity.
	for len(c.mu.entries) >= c.maxSize {
		c.evictLRULocked()
	}

	// Create new entry.
	entry := &CacheEntry{
		Key:          key,
		Memo:         m,
		PhysProps:    physProps,
		ValidSince:   time.Now(),
		Dependencies: deps,
		Stats: PlanStats{
			LastUsed: time.Now(),
		},
	}
	entry.lruElement = c.mu.lru.PushFront(entry)
	c.mu.entries[hash] = entry

	if c.metrics != nil {
		c.metrics.Size.Update(int64(len(c.mu.entries)))
	}
}

// evictLRULocked removes the least recently used entry.
// Caller must hold c.mu.Lock().
func (c *ClusterPlanCache) evictLRULocked() {
	if c.mu.lru.Len() == 0 {
		return
	}

	// Get the back element (LRU).
	elem := c.mu.lru.Back()
	if elem == nil {
		return
	}

	entry := elem.Value.(*CacheEntry)
	c.mu.lru.Remove(elem)
	delete(c.mu.entries, entry.Key.Hash())

	if c.metrics != nil {
		c.metrics.Evictions.Inc(1)
		c.metrics.Size.Update(int64(len(c.mu.entries)))
	}
}

// InvalidateTable removes all entries that depend on the given table.
func (c *ClusterPlanCache) InvalidateTable(tableID descpb.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for hash, entry := range c.mu.entries {
		for _, dep := range entry.Dependencies {
			if dep == tableID {
				c.mu.lru.Remove(entry.lruElement)
				delete(c.mu.entries, hash)
				if c.metrics != nil {
					c.metrics.Evictions.Inc(1)
				}
				break
			}
		}
	}
	if c.metrics != nil {
		c.metrics.Size.Update(int64(len(c.mu.entries)))
	}
}

// Clear removes all entries from the cache.
func (c *ClusterPlanCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.mu.entries = make(map[string]*CacheEntry)
	c.mu.lru.Init()
	if c.metrics != nil {
		c.metrics.Size.Update(0)
	}
}

// Len returns the number of entries in the cache.
func (c *ClusterPlanCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.mu.entries)
}

// GetEntries returns a copy of all cache entries for inspection.
func (c *ClusterPlanCache) GetEntries() []*CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]*CacheEntry, 0, len(c.mu.entries))
	for _, entry := range c.mu.entries {
		entries = append(entries, entry)
	}
	return entries
}

// PlanCacheProvider is an interface for accessing plan caches.
type PlanCacheProvider interface {
	// SessionCache returns the session-level plan cache.
	SessionCache() *SessionPlanCache

	// ClusterCache returns the cluster-level plan cache.
	ClusterCache() *ClusterPlanCache

	// IsEnabled returns whether plan caching is enabled.
	IsEnabled() bool
}

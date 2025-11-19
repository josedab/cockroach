// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package plancache

import (
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/sql/catalog/descpb"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func TestCacheKeyHash(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	key1 := CacheKey{
		Fingerprint:   "SELECT * FROM users WHERE id = $1",
		Database:      "mydb",
		SearchPath:    "public",
		DistSQLMode:   "auto",
		VectorizeMode: "on",
	}

	key2 := CacheKey{
		Fingerprint:   "SELECT * FROM users WHERE id = $1",
		Database:      "mydb",
		SearchPath:    "public",
		DistSQLMode:   "auto",
		VectorizeMode: "on",
	}

	// Same keys should produce same hash.
	require.Equal(t, key1.Hash(), key2.Hash())

	// Different fingerprint should produce different hash.
	key3 := key1
	key3.Fingerprint = "SELECT * FROM orders WHERE id = $1"
	require.NotEqual(t, key1.Hash(), key3.Hash())

	// Different database should produce different hash.
	key4 := key1
	key4.Database = "otherdb"
	require.NotEqual(t, key1.Hash(), key4.Hash())
}

func TestSessionPlanCacheLookupAndInsert(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	metrics := NewMetrics()
	cache := NewSessionPlanCache(100, metrics)

	key := CacheKey{
		Fingerprint: "SELECT * FROM users WHERE id = $1",
		Database:    "mydb",
	}

	// Cache miss on empty cache.
	entry, ok := cache.Lookup(key)
	require.False(t, ok)
	require.Nil(t, entry)

	// Insert an entry.
	deps := []descpb.ID{1, 2}
	cache.Insert(key, nil, nil, deps)

	// Cache hit.
	entry, ok = cache.Lookup(key)
	require.True(t, ok)
	require.NotNil(t, entry)
	require.Equal(t, key.Fingerprint, entry.Key.Fingerprint)
	require.Equal(t, deps, entry.Dependencies)
	require.Equal(t, int64(1), entry.Stats.HitCount)

	// Second lookup increments hit count.
	entry, ok = cache.Lookup(key)
	require.True(t, ok)
	require.Equal(t, int64(2), entry.Stats.HitCount)
}

func TestSessionPlanCacheLRUEviction(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	cache := NewSessionPlanCache(3, nil)

	// Insert 3 entries.
	for i := 0; i < 3; i++ {
		key := CacheKey{
			Fingerprint: string(rune('a' + i)),
			Database:    "mydb",
		}
		cache.Insert(key, nil, nil, nil)
	}
	require.Equal(t, 3, cache.Len())

	// Access first entry to make it recently used.
	key0 := CacheKey{Fingerprint: "a", Database: "mydb"}
	_, ok := cache.Lookup(key0)
	require.True(t, ok)

	// Insert 4th entry, should evict LRU (key "b").
	key3 := CacheKey{Fingerprint: "d", Database: "mydb"}
	cache.Insert(key3, nil, nil, nil)
	require.Equal(t, 3, cache.Len())

	// key "a" and "d" should exist, "b" should be evicted.
	_, ok = cache.Lookup(key0)
	require.True(t, ok)

	_, ok = cache.Lookup(key3)
	require.True(t, ok)

	key1 := CacheKey{Fingerprint: "b", Database: "mydb"}
	_, ok = cache.Lookup(key1)
	require.False(t, ok)

	// key "c" should still exist (wasn't the LRU).
	key2 := CacheKey{Fingerprint: "c", Database: "mydb"}
	_, ok = cache.Lookup(key2)
	require.True(t, ok)
}

func TestSessionPlanCacheInvalidateTable(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	cache := NewSessionPlanCache(100, nil)

	// Insert entries with different dependencies.
	key1 := CacheKey{Fingerprint: "query1", Database: "mydb"}
	cache.Insert(key1, nil, nil, []descpb.ID{1, 2})

	key2 := CacheKey{Fingerprint: "query2", Database: "mydb"}
	cache.Insert(key2, nil, nil, []descpb.ID{2, 3})

	key3 := CacheKey{Fingerprint: "query3", Database: "mydb"}
	cache.Insert(key3, nil, nil, []descpb.ID{4})

	require.Equal(t, 3, cache.Len())

	// Invalidate table 2, should remove key1 and key2.
	cache.InvalidateTable(2)
	require.Equal(t, 1, cache.Len())

	_, ok := cache.Lookup(key1)
	require.False(t, ok)

	_, ok = cache.Lookup(key2)
	require.False(t, ok)

	_, ok = cache.Lookup(key3)
	require.True(t, ok)
}

func TestSessionPlanCacheClear(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	cache := NewSessionPlanCache(100, nil)

	// Insert entries.
	for i := 0; i < 10; i++ {
		key := CacheKey{
			Fingerprint: string(rune('a' + i)),
			Database:    "mydb",
		}
		cache.Insert(key, nil, nil, nil)
	}
	require.Equal(t, 10, cache.Len())

	// Clear should remove all entries.
	cache.Clear()
	require.Equal(t, 0, cache.Len())
}

func TestClusterPlanCacheLookupAndInsert(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	metrics := NewMetrics()
	cache := NewClusterPlanCache(100, metrics)

	key := CacheKey{
		Fingerprint: "SELECT * FROM users WHERE id = $1",
		Database:    "mydb",
	}

	// Cache miss on empty cache.
	entry, ok := cache.Lookup(key)
	require.False(t, ok)
	require.Nil(t, entry)

	// Insert an entry.
	deps := []descpb.ID{1, 2}
	cache.Insert(key, nil, nil, deps)

	// Cache hit.
	entry, ok = cache.Lookup(key)
	require.True(t, ok)
	require.NotNil(t, entry)
	require.Equal(t, key.Fingerprint, entry.Key.Fingerprint)
	require.Equal(t, deps, entry.Dependencies)
	require.Equal(t, int64(1), entry.Stats.HitCount)
}

func TestClusterPlanCacheInvalidateTable(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	cache := NewClusterPlanCache(100, nil)

	// Insert entries with different dependencies.
	key1 := CacheKey{Fingerprint: "query1", Database: "mydb"}
	cache.Insert(key1, nil, nil, []descpb.ID{1, 2})

	key2 := CacheKey{Fingerprint: "query2", Database: "mydb"}
	cache.Insert(key2, nil, nil, []descpb.ID{3})

	require.Equal(t, 2, cache.Len())

	// Invalidate table 1, should remove key1.
	cache.InvalidateTable(1)
	require.Equal(t, 1, cache.Len())

	_, ok := cache.Lookup(key1)
	require.False(t, ok)

	_, ok = cache.Lookup(key2)
	require.True(t, ok)
}

func TestInvalidatorSchemaChange(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	sessionCache := NewSessionPlanCache(100, nil)
	clusterCache := NewClusterPlanCache(100, nil)
	inv := NewInvalidator(sessionCache, clusterCache, time.Hour)

	// Insert entries.
	key1 := CacheKey{Fingerprint: "query1", Database: "mydb"}
	sessionCache.Insert(key1, nil, nil, []descpb.ID{1})
	clusterCache.Insert(key1, nil, nil, []descpb.ID{1})

	key2 := CacheKey{Fingerprint: "query2", Database: "mydb"}
	sessionCache.Insert(key2, nil, nil, []descpb.ID{2})
	clusterCache.Insert(key2, nil, nil, []descpb.ID{2})

	// Invalidate for schema change on table 1.
	event := inv.InvalidateForSchemaChange(1)
	require.Equal(t, InvalidationSchemaChange, event.Reason)
	require.Equal(t, descpb.ID(1), event.TableID)
	require.Equal(t, 2, event.EntriesInvalidated) // 1 from each cache

	// key1 should be gone from both caches.
	_, ok := sessionCache.Lookup(key1)
	require.False(t, ok)

	_, ok = clusterCache.Lookup(key1)
	require.False(t, ok)

	// key2 should still exist.
	_, ok = sessionCache.Lookup(key2)
	require.True(t, ok)

	_, ok = clusterCache.Lookup(key2)
	require.True(t, ok)
}

func TestInvalidatorTTL(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	sessionCache := NewSessionPlanCache(100, nil)
	inv := NewInvalidator(sessionCache, nil, 100*time.Millisecond)

	// Insert an entry.
	key := CacheKey{Fingerprint: "query1", Database: "mydb"}
	sessionCache.Insert(key, nil, nil, nil)

	// Entry should be valid immediately.
	require.True(t, inv.IsValid(sessionCache.entries[key.Hash()]))

	// Wait for TTL to expire.
	time.Sleep(150 * time.Millisecond)

	// Entry should be invalid now.
	require.False(t, inv.IsValid(sessionCache.entries[key.Hash()]))

	// InvalidateExpired should remove it.
	event := inv.InvalidateExpired()
	require.Equal(t, InvalidationTTLExpired, event.Reason)
	require.Equal(t, 1, event.EntriesInvalidated)
	require.Equal(t, 0, sessionCache.Len())
}

func TestInvalidatorClearAll(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	sessionCache := NewSessionPlanCache(100, nil)
	clusterCache := NewClusterPlanCache(100, nil)
	inv := NewInvalidator(sessionCache, clusterCache, time.Hour)

	// Insert entries.
	for i := 0; i < 5; i++ {
		key := CacheKey{
			Fingerprint: string(rune('a' + i)),
			Database:    "mydb",
		}
		sessionCache.Insert(key, nil, nil, nil)
		clusterCache.Insert(key, nil, nil, nil)
	}

	require.Equal(t, 5, sessionCache.Len())
	require.Equal(t, 5, clusterCache.Len())

	// Clear all.
	event := inv.ClearAll()
	require.Equal(t, InvalidationManual, event.Reason)
	require.Equal(t, 10, event.EntriesInvalidated)
	require.Equal(t, 0, sessionCache.Len())
	require.Equal(t, 0, clusterCache.Len())
}

func TestMetrics(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	metrics := NewMetrics()
	cache := NewSessionPlanCache(2, metrics)

	key1 := CacheKey{Fingerprint: "query1", Database: "mydb"}
	key2 := CacheKey{Fingerprint: "query2", Database: "mydb"}

	// Miss.
	cache.Lookup(key1)
	require.Equal(t, int64(1), metrics.Misses.Count())

	// Insert.
	cache.Insert(key1, nil, nil, nil)
	require.Equal(t, int64(1), metrics.Size.Value())

	// Hit.
	cache.Lookup(key1)
	require.Equal(t, int64(1), metrics.Hits.Count())

	// Insert second entry.
	cache.Insert(key2, nil, nil, nil)
	require.Equal(t, int64(2), metrics.Size.Value())

	// Insert third entry, should evict.
	key3 := CacheKey{Fingerprint: "query3", Database: "mydb"}
	cache.Insert(key3, nil, nil, nil)
	require.Equal(t, int64(1), metrics.Evictions.Count())
	require.Equal(t, int64(2), metrics.Size.Value())
}

func TestGetEntries(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	cache := NewSessionPlanCache(100, nil)

	// Insert entries.
	for i := 0; i < 3; i++ {
		key := CacheKey{
			Fingerprint: string(rune('a' + i)),
			Database:    "mydb",
		}
		cache.Insert(key, nil, nil, nil)
	}

	entries := cache.GetEntries()
	require.Len(t, entries, 3)

	// Verify all fingerprints are present.
	fingerprints := make(map[string]bool)
	for _, entry := range entries {
		fingerprints[entry.Key.Fingerprint] = true
	}
	require.True(t, fingerprints["a"])
	require.True(t, fingerprints["b"])
	require.True(t, fingerprints["c"])
}

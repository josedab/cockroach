// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package plancache

import (
	"time"

	"github.com/cockroachdb/cockroach/pkg/settings"
)

// Enabled controls whether the query plan cache is enabled.
var Enabled = settings.RegisterBoolSetting(
	settings.ApplicationLevel,
	"sql.plan_cache.enabled",
	"enable query plan caching for the optimizer (experimental)",
	false, // Default disabled for safety
	settings.WithPublic,
)

// SessionCacheSize controls the maximum number of plans cached per session.
var SessionCacheSize = settings.RegisterIntSetting(
	settings.ApplicationLevel,
	"sql.plan_cache.session_size",
	"maximum number of query plans to cache per session",
	100,
	settings.PositiveInt,
	settings.WithPublic,
)

// ClusterCacheSize controls the maximum number of plans in the shared cluster cache.
var ClusterCacheSize = settings.RegisterIntSetting(
	settings.ApplicationLevel,
	"sql.plan_cache.cluster_size",
	"maximum number of query plans in the shared cluster cache",
	10000,
	settings.PositiveInt,
	settings.WithPublic,
)

// TTL controls how long plans are kept in the cache before being invalidated.
var TTL = settings.RegisterDurationSetting(
	settings.ApplicationLevel,
	"sql.plan_cache.ttl",
	"time-to-live for cached query plans (0 to disable TTL-based invalidation)",
	time.Hour,
	settings.WithPublic,
)

// UseClusterCache controls whether to use the shared cluster cache
// in addition to session caches.
var UseClusterCache = settings.RegisterBoolSetting(
	settings.ApplicationLevel,
	"sql.plan_cache.use_cluster_cache",
	"enable the shared cluster-level plan cache",
	false, // Default disabled, session cache only
	settings.WithPublic,
)

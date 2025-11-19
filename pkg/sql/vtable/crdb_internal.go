// Copyright 2023 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package vtable

// CrdbInternalBuiltinFunctionComments describes the schema of the
// crdb_internal.kv_builtin_function_comments table.
var CrdbInternalBuiltinFunctionComments = `
CREATE TABLE crdb_internal.kv_builtin_function_comments (
  oid         OID NOT NULL,
  description STRING NOT NULL
)`

// CrdbInternalCatalogComments describes the schema of the
// crdb_internal.kv_catalog_comments table.
var CrdbInternalCatalogComments = `
CREATE TABLE crdb_internal.kv_catalog_comments (
  classoid    OID NOT NULL,
  objoid      OID NOT NULL,
  objsubid    INT4 NOT NULL,
  description STRING NOT NULL
)`

// CrdbInternalPlanCache describes the schema of the
// crdb_internal.plan_cache table.
var CrdbInternalPlanCache = `
CREATE TABLE crdb_internal.plan_cache (
  fingerprint        STRING NOT NULL,
  database_name      STRING NOT NULL,
  hit_count          INT8 NOT NULL,
  valid_since        TIMESTAMP NOT NULL,
  last_used          TIMESTAMP NOT NULL,
  dependency_ids     INT8[] NOT NULL
)`

// CrdbInternalPlanCacheStats describes the schema of the
// crdb_internal.plan_cache_stats table.
var CrdbInternalPlanCacheStats = `
CREATE TABLE crdb_internal.plan_cache_stats (
  cache_type         STRING NOT NULL,
  total_entries      INT8 NOT NULL,
  total_hits         INT8 NOT NULL,
  total_misses       INT8 NOT NULL,
  total_evictions    INT8 NOT NULL,
  memory_bytes       INT8 NOT NULL
)`

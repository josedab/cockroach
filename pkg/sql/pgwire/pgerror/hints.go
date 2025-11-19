// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package pgerror

import (
	"strings"

	"github.com/cockroachdb/cockroach/pkg/docs"
	"github.com/cockroachdb/cockroach/pkg/settings"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
)

// EnhancedHintsEnabled controls whether enhanced error hints are enabled.
// When disabled, only basic error messages are shown without additional
// troubleshooting guidance.
var EnhancedHintsEnabled = settings.RegisterBoolSetting(
	settings.ApplicationLevel,
	"sql.errors.enhanced_hints.enabled",
	"when true, error messages include detailed troubleshooting hints and documentation links",
	true,
	settings.WithPublic,
)

// HintTemplate defines a template for error hints that can include
// static content and documentation links.
type HintTemplate struct {
	// Hint is the troubleshooting guidance text
	Hint string
	// DocLink is the URL to relevant documentation
	DocLink string
}

// hintRegistry maps PostgreSQL error codes to their hint templates.
// These hints provide actionable troubleshooting guidance for common errors.
var hintRegistry = map[pgcode.Code]HintTemplate{
	// Transaction errors (40001)
	pgcode.SerializationFailure: {
		Hint: `Transaction conflicted with another and must be retried.

Common causes:
  - High contention on hot keys
  - Long-running transactions

Troubleshooting:
  - View contention: SELECT * FROM crdb_internal.cluster_contention_events LIMIT 10;
  - Check transaction duration: SELECT * FROM crdb_internal.cluster_queries;
  - Consider adding retry logic in your application`,
		DocLink: "transaction-retry-error-reference.html",
	},

	// Query timeout errors (57014)
	pgcode.QueryCanceled: {
		Hint: `Query was canceled, typically due to timeout or user interruption.

Common causes:
  - Statement timeout exceeded
  - Query canceled by administrator
  - Client disconnection

Troubleshooting:
  - Check current timeout: SHOW statement_timeout;
  - Increase timeout if needed: SET statement_timeout = '5m';
  - Optimize query to reduce execution time
  - Check for blocking operations: SELECT * FROM crdb_internal.cluster_locks;`,
		DocLink: "show-vars.html#statement_timeout",
	},

	// Permission errors (42501)
	pgcode.InsufficientPrivilege: {
		Hint: `The current user does not have the required privileges for this operation.

Troubleshooting:
  - Check current user: SELECT current_user();
  - List user privileges: SHOW GRANTS FOR <username>;
  - Grant required privileges: GRANT <privilege> ON <object> TO <user>;
  - Check role membership: SHOW GRANTS ON ROLE FOR <username>;`,
		DocLink: "security-reference/authorization.html#privileges",
	},

	// Undefined table (42P01)
	pgcode.UndefinedTable: {
		Hint: `The specified table or relation does not exist.

Troubleshooting:
  - List tables in current schema: SHOW TABLES;
  - Check if table exists in another schema: SHOW TABLES FROM <schema>;
  - Verify search path: SHOW search_path;
  - Check spelling and case sensitivity of table name`,
		DocLink: "show-tables.html",
	},

	// Configuration limit exceeded (53400)
	pgcode.ConfigurationLimitExceeded: {
		Hint: `A system configuration limit was exceeded.

Common causes:
  - Memory budget exceeded for query
  - Too many concurrent connections
  - Disk space limitations

Troubleshooting:
  - Check current memory usage: SELECT * FROM crdb_internal.node_memory_monitors;
  - View running queries: SELECT * FROM crdb_internal.cluster_queries WHERE status = 'running';
  - Reduce result set size with LIMIT
  - Add indexes to avoid full table scans`,
		DocLink: "cluster-settings.html",
	},

	// Connection errors (08000)
	pgcode.ConnectionException: {
		Hint: `A connection error occurred.

Common causes:
  - Network connectivity issues
  - Server unavailable
  - Connection pool exhausted

Troubleshooting:
  - Verify server is running and accessible
  - Check network connectivity to cluster
  - Review connection pool settings
  - Check for firewall rules blocking connections`,
		DocLink: "connection-parameters.html",
	},

	// Deadlock detected (40P01)
	pgcode.DeadlockDetected: {
		Hint: `A deadlock was detected between concurrent transactions.

Troubleshooting:
  - Review transaction ordering in your application
  - Consider using SELECT FOR UPDATE to acquire locks in consistent order
  - Reduce transaction scope and duration
  - View contention events: SELECT * FROM crdb_internal.cluster_contention_events;`,
		DocLink: "performance-best-practices-overview.html#transaction-contention",
	},

	// Out of memory (53200)
	pgcode.OutOfMemory: {
		Hint: `Query exceeded memory allocation.

Immediate actions:
  - Reduce result set size with LIMIT
  - Add index to avoid full table scan
  - Break query into smaller batches

Diagnostics:
  - Check memory usage: SELECT * FROM crdb_internal.node_memory_monitors;
  - View running queries: SELECT * FROM crdb_internal.cluster_queries WHERE status = 'running';
  - Consider increasing distsql_workmem: SET distsql_workmem = '256MiB';`,
		DocLink: "cluster-settings.html#sql-defaults-distsql-workmem",
	},

	// Disk full (53100)
	pgcode.DiskFull: {
		Hint: `The disk is full and cannot accept more data.

Immediate actions:
  - Free up disk space by removing unnecessary files
  - Check for large temporary files
  - Consider adding storage capacity

Diagnostics:
  - Check store capacity in the Admin UI
  - Review data distribution across nodes`,
		DocLink: "monitoring-and-alerting.html#disk-capacity",
	},
}

// GetHintForCode retrieves the hint template for a given error code.
// Returns an empty HintTemplate if no hint is registered for the code.
func GetHintForCode(code pgcode.Code) HintTemplate {
	if template, ok := hintRegistry[code]; ok {
		return template
	}
	return HintTemplate{}
}

// FormatHintWithDocLink formats the hint template into a string that includes
// the hint text and documentation link.
func FormatHintWithDocLink(template HintTemplate) string {
	if template.Hint == "" {
		return ""
	}

	var result strings.Builder
	result.WriteString(template.Hint)

	if template.DocLink != "" {
		result.WriteString("\n\nDocs: ")
		result.WriteString(docs.URL(template.DocLink))
	}

	return result.String()
}

// EnhanceErrorWithHint adds a hint to an error based on its PostgreSQL error code
// if enhanced hints are enabled and a hint template exists for the code.
// The sv parameter should be nil when settings are not available; in that case,
// hints will be added based on the default setting value.
func EnhanceErrorWithHint(err error, sv *settings.Values) error {
	if err == nil {
		return nil
	}

	// Check if enhanced hints are enabled
	if sv != nil && !EnhancedHintsEnabled.Get(sv) {
		return err
	}

	code := GetPGCode(err)
	template := GetHintForCode(code)

	if template.Hint == "" {
		return err
	}

	// Format the hint with documentation link
	formattedHint := FormatHintWithDocLink(template)
	if formattedHint == "" {
		return err
	}

	return err
}

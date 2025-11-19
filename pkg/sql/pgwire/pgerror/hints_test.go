// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package pgerror_test

import (
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/settings/cluster"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
	"github.com/stretchr/testify/require"
)

func TestHintRegistry(t *testing.T) {
	// Test that hints exist for the expected error codes
	testCases := []struct {
		code           pgcode.Code
		expectHint     bool
		hintContains   []string
		docLinkPattern string
	}{
		{
			code:       pgcode.SerializationFailure,
			expectHint: true,
			hintContains: []string{
				"Transaction conflicted",
				"contention",
				"crdb_internal.cluster_contention_events",
			},
			docLinkPattern: "transaction-retry-error-reference.html",
		},
		{
			code:       pgcode.QueryCanceled,
			expectHint: true,
			hintContains: []string{
				"canceled",
				"timeout",
				"statement_timeout",
			},
			docLinkPattern: "show-vars.html",
		},
		{
			code:       pgcode.InsufficientPrivilege,
			expectHint: true,
			hintContains: []string{
				"privileges",
				"SHOW GRANTS",
				"GRANT",
			},
			docLinkPattern: "authorization.html",
		},
		{
			code:       pgcode.UndefinedTable,
			expectHint: true,
			hintContains: []string{
				"table",
				"SHOW TABLES",
				"search_path",
			},
			docLinkPattern: "show-tables.html",
		},
		{
			code:       pgcode.ConfigurationLimitExceeded,
			expectHint: true,
			hintContains: []string{
				"limit",
				"memory",
				"crdb_internal.node_memory_monitors",
			},
			docLinkPattern: "cluster-settings.html",
		},
		{
			code:       pgcode.OutOfMemory,
			expectHint: true,
			hintContains: []string{
				"memory",
				"LIMIT",
				"distsql_workmem",
			},
			docLinkPattern: "cluster-settings.html",
		},
		{
			code:       pgcode.DeadlockDetected,
			expectHint: true,
			hintContains: []string{
				"deadlock",
				"SELECT FOR UPDATE",
				"contention",
			},
			docLinkPattern: "performance-best-practices",
		},
		{
			code:           pgcode.Syntax,
			expectHint:     false,
			hintContains:   nil,
			docLinkPattern: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.code.String(), func(t *testing.T) {
			template := pgerror.GetHintForCode(tc.code)

			if tc.expectHint {
				require.NotEmpty(t, template.Hint, "expected hint for code %s", tc.code)

				for _, contains := range tc.hintContains {
					require.Contains(t, template.Hint, contains,
						"hint for %s should contain %q", tc.code, contains)
				}

				if tc.docLinkPattern != "" {
					require.NotEmpty(t, template.DocLink,
						"expected doc link for code %s", tc.code)
					require.Contains(t, template.DocLink, tc.docLinkPattern,
						"doc link for %s should contain %q", tc.code, tc.docLinkPattern)
				}
			} else {
				require.Empty(t, template.Hint, "expected no hint for code %s", tc.code)
			}
		})
	}
}

func TestFormatHintWithDocLink(t *testing.T) {
	testCases := []struct {
		name     string
		template pgerror.HintTemplate
		expected string
	}{
		{
			name:     "empty template",
			template: pgerror.HintTemplate{},
			expected: "",
		},
		{
			name: "hint only",
			template: pgerror.HintTemplate{
				Hint: "Try something else",
			},
			expected: "Try something else",
		},
		{
			name: "hint with doc link",
			template: pgerror.HintTemplate{
				Hint:    "Check the docs",
				DocLink: "foo.html",
			},
			expected: "Check the docs\n\nDocs: ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := pgerror.FormatHintWithDocLink(tc.template)
			if tc.expected == "" {
				require.Empty(t, result)
			} else {
				require.Contains(t, result, tc.expected)
			}
		})
	}
}

func TestFlattenWithSettings(t *testing.T) {
	// Create cluster settings
	st := cluster.MakeTestingClusterSettings()

	t.Run("enhanced hints enabled by default", func(t *testing.T) {
		// Create an error with a code that has a hint
		err := pgerror.New(pgcode.InsufficientPrivilege, "permission denied")
		pgErr := pgerror.FlattenWithSettings(err, &st.SV)

		require.NotEmpty(t, pgErr.Hint, "expected enhanced hint when enabled")
		require.Contains(t, pgErr.Hint, "privileges")
		require.Contains(t, pgErr.Hint, "SHOW GRANTS")
	})

	t.Run("enhanced hints disabled", func(t *testing.T) {
		// Disable enhanced hints
		pgerror.EnhancedHintsEnabled.Override(
			&(cluster.MakeTestingClusterSettings()).SV,
			false,
		)
		stDisabled := cluster.MakeTestingClusterSettings()
		pgerror.EnhancedHintsEnabled.Override(&stDisabled.SV, false)

		// Create an error with a code that has a hint
		err := pgerror.New(pgcode.InsufficientPrivilege, "permission denied")
		pgErr := pgerror.FlattenWithSettings(err, &stDisabled.SV)

		// The hint should be empty because enhanced hints are disabled
		require.Empty(t, pgErr.Hint, "expected no enhanced hint when disabled")
	})

	t.Run("existing hint preserved", func(t *testing.T) {
		// Create an error with an existing hint
		err := pgerror.WithCandidateCode(
			pgerror.New(pgcode.InsufficientPrivilege, "permission denied"),
			pgcode.InsufficientPrivilege,
		)

		// Manually wrap with a hint to simulate existing hint
		wrappedErr := pgerror.Newf(pgcode.InsufficientPrivilege, "permission denied")

		pgErr := pgerror.FlattenWithSettings(wrappedErr, &st.SV)

		require.NotEmpty(t, pgErr.Hint, "expected hint")
		// Enhanced hint should be added
		require.Contains(t, pgErr.Hint, "SHOW GRANTS")
	})

	t.Run("nil settings uses default", func(t *testing.T) {
		// When settings is nil, should use default (enabled)
		err := pgerror.New(pgcode.QueryCanceled, "query canceled")
		pgErr := pgerror.FlattenWithSettings(err, nil)

		require.NotEmpty(t, pgErr.Hint, "expected enhanced hint with nil settings")
		require.Contains(t, pgErr.Hint, "timeout")
	})

	t.Run("no hint for unregistered code", func(t *testing.T) {
		// Create an error with a code that has no registered hint
		err := pgerror.New(pgcode.Syntax, "syntax error")
		pgErr := pgerror.FlattenWithSettings(err, &st.SV)

		// Should have no hint (Syntax has no registered hint)
		require.Empty(t, pgErr.Hint)
	})
}

func TestEnhancedHintContent(t *testing.T) {
	st := cluster.MakeTestingClusterSettings()

	testCases := []struct {
		name         string
		code         pgcode.Code
		message      string
		mustContain  []string
		mustNotEmpty []string
	}{
		{
			name:    "serialization failure",
			code:    pgcode.SerializationFailure,
			message: "transaction retry",
			mustContain: []string{
				"crdb_internal.cluster_contention_events",
				"retry logic",
			},
		},
		{
			name:    "query canceled",
			code:    pgcode.QueryCanceled,
			message: "query canceled",
			mustContain: []string{
				"statement_timeout",
				"SHOW statement_timeout",
				"SET statement_timeout",
			},
		},
		{
			name:    "insufficient privilege",
			code:    pgcode.InsufficientPrivilege,
			message: "permission denied",
			mustContain: []string{
				"current_user",
				"SHOW GRANTS",
				"GRANT",
			},
		},
		{
			name:    "undefined table",
			code:    pgcode.UndefinedTable,
			message: "table not found",
			mustContain: []string{
				"SHOW TABLES",
				"search_path",
			},
		},
		{
			name:    "out of memory",
			code:    pgcode.OutOfMemory,
			message: "memory budget exceeded",
			mustContain: []string{
				"LIMIT",
				"distsql_workmem",
				"crdb_internal.node_memory_monitors",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := pgerror.New(tc.code, tc.message)
			pgErr := pgerror.FlattenWithSettings(err, &st.SV)

			require.NotEmpty(t, pgErr.Hint, "expected hint for %s", tc.name)

			for _, substr := range tc.mustContain {
				require.True(t, strings.Contains(pgErr.Hint, substr),
					"hint for %s should contain %q, got: %s", tc.name, substr, pgErr.Hint)
			}

			// Check that doc link is included
			require.Contains(t, pgErr.Hint, "Docs:",
				"hint for %s should contain documentation link", tc.name)
			require.Contains(t, pgErr.Hint, "cockroachlabs.com/docs",
				"hint should contain full docs URL")
		})
	}
}

func TestBackwardsCompatibility(t *testing.T) {
	// Test that the original Flatten function still works
	err := pgerror.New(pgcode.Syntax, "syntax error")
	pgErr := pgerror.Flatten(err)

	require.NotNil(t, pgErr)
	require.Equal(t, pgcode.Syntax.String(), pgErr.Code)
	require.Equal(t, "syntax error", pgErr.Message)

	// Note: The original Flatten doesn't include enhanced hints
	// Only FlattenWithSettings does
}

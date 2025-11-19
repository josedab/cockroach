// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package main

import (
	"github.com/spf13/cobra"
)

// makeQuickTestCmd creates the `qt` (quick test) command
func makeQuickTestCmd(runE func(cmd *cobra.Command, args []string) error) *cobra.Command {
	qtCmd := &cobra.Command{
		Use:   "qt [pkg...]",
		Short: "Quick test - run tests for changed files",
		Long: `Quick test runs tests for files that have changed since the last commit.

This is a shortcut for "dev test --changed -v".

It automatically determines which tests to run based on git diff analysis,
making it ideal for rapid iteration during development.`,
		Example: `
    dev qt
        Run tests for all changed files

    dev qt pkg/sql
        Run tests for changed files in pkg/sql only

    dev qt --filter=TestParse
        Run specific test pattern for changed files`,
		Args: cobra.MinimumNArgs(0),
		RunE: runE,
	}
	addCommonBuildFlags(qtCmd)
	addCommonTestFlags(qtCmd)
	// Add test-specific flags that might be needed
	qtCmd.Flags().Bool(changedFlag, true, "automatically determine tests to run (always true for qt)")
	qtCmd.Flags().BoolP(vFlag, "v", true, "show testing process output (always true for qt)")
	qtCmd.Flags().Int(countFlag, 0, "run test the given number of times")
	qtCmd.Flags().BoolP(showLogsFlag, "", false, "show crdb logs in-line")
	qtCmd.Flags().Bool(stressFlag, false, "run tests under stress")
	qtCmd.Flags().Bool(raceFlag, false, "run tests using race builds")
	qtCmd.Flags().Bool(deadlockFlag, false, "run tests using the deadlock detector")
	qtCmd.Flags().Bool(ignoreCacheFlag, false, "ignore cached test runs")
	qtCmd.Flags().Bool(rewriteFlag, false, "rewrite test files using results from test run")
	qtCmd.Flags().Bool(streamOutputFlag, false, "stream test output during run")
	qtCmd.Flags().String(testArgsFlag, "", "additional arguments to pass to the go test binary")
	qtCmd.Flags().String(vModuleFlag, "", "comma-separated list of pattern=N settings for file-filtered logging")
	qtCmd.Flags().Bool(showDiffFlag, false, "generate a diff for expectation mismatches when possible")
	qtCmd.Flags().Bool(watchFlag, false, "watch for file changes and re-run tests automatically")
	qtCmd.Flags().String(profileFlag, "", "use a predefined test profile (fast, thorough, stress, ci)")
	qtCmd.Flags().Bool(prioritizeFlag, false, "run likely-to-fail tests first")
	return qtCmd
}

// makeQuickBuildCmd creates the `qb` (quick build) command
func makeQuickBuildCmd(runE func(cmd *cobra.Command, args []string) error) *cobra.Command {
	qbCmd := &cobra.Command{
		Use:   "qb [targets...]",
		Short: "Quick build - build affected packages",
		Long: `Quick build compiles packages that have changed since the last commit.

This is a shortcut for building affected targets based on git diff.`,
		Example: `
    dev qb
        Build cockroach binary (default)

    dev qb short
        Build cockroach-short binary`,
		Args: cobra.MinimumNArgs(0),
		RunE: runE,
	}
	addCommonBuildFlags(qbCmd)
	return qbCmd
}

// makeQuickLintCmd creates the `ql` (quick lint) command
func makeQuickLintCmd(runE func(cmd *cobra.Command, args []string) error) *cobra.Command {
	qlCmd := &cobra.Command{
		Use:   "ql [pkg]",
		Short: "Quick lint - lint changed files only",
		Long: `Quick lint runs linters only on files that have changed.

This is a shortcut for "dev lint --short" which runs fast linters.`,
		Example: `
    dev ql
        Run fast linters on all changed files

    dev ql pkg/sql
        Run fast linters on pkg/sql`,
		Args: cobra.MinimumNArgs(0),
		RunE: runE,
	}
	addCommonBuildFlags(qlCmd)
	addCommonTestFlags(qlCmd)
	// Set short to true by default for quick lint
	qlCmd.Flags().Lookup(shortFlag).DefValue = "true"
	qlCmd.Flags().Lookup(shortFlag).Value.Set("true")
	return qlCmd
}

// quickTest implements the qt command
func (d *dev) quickTest(cmd *cobra.Command, args []string) error {
	// Qt is essentially "dev test --changed -v" but the flags are already set
	// with defaults in makeQuickTestCmd, so we just call test directly
	return d.test(cmd, args)
}

// quickBuild implements the qb command
func (d *dev) quickBuild(cmd *cobra.Command, args []string) error {
	// Default to short build for quick builds
	if len(args) == 0 {
		args = []string{"short"}
	}
	return d.build(cmd, args)
}

// quickLint implements the ql command
func (d *dev) quickLint(cmd *cobra.Command, args []string) error {
	// Ql is essentially "dev lint --short" but the flag is already set
	// with default in makeQuickLintCmd, so we just call lint directly
	return d.lint(cmd, args)
}

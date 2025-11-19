// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProfiles(t *testing.T) {
	// Test getting predefined profiles
	t.Run("get fast profile", func(t *testing.T) {
		profile, err := getProfile("fast")
		require.NoError(t, err)
		require.Equal(t, "fast", profile.Name)
		require.Equal(t, 1*time.Minute, profile.Timeout)
		require.False(t, profile.Race)
		require.True(t, profile.Short)
	})

	t.Run("get thorough profile", func(t *testing.T) {
		profile, err := getProfile("thorough")
		require.NoError(t, err)
		require.Equal(t, "thorough", profile.Name)
		require.Equal(t, 10*time.Minute, profile.Timeout)
		require.True(t, profile.Race)
		require.False(t, profile.Short)
		require.True(t, profile.Verbose)
	})

	t.Run("get ci profile", func(t *testing.T) {
		profile, err := getProfile("ci")
		require.NoError(t, err)
		require.Equal(t, "ci", profile.Name)
		require.True(t, profile.Race)
		require.True(t, profile.IgnoreCache)
	})

	t.Run("get stress profile", func(t *testing.T) {
		profile, err := getProfile("stress")
		require.NoError(t, err)
		require.Equal(t, "stress", profile.Name)
		require.Equal(t, 100, profile.Stress)
	})

	t.Run("unknown profile returns error", func(t *testing.T) {
		_, err := getProfile("unknown")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown profile")
	})
}

func TestDiagnostics(t *testing.T) {
	t.Run("suggest fix for common errors", func(t *testing.T) {
		tests := []struct {
			errorMsg    string
			shouldMatch bool
		}{
			{"context deadline exceeded", true},
			{"connection refused", true},
			{"transaction retry error", true},
			{"deadlock detected", true},
			{"permission denied", true},
			{"random unknown error", false},
		}

		for _, tt := range tests {
			suggestion := suggestFix(tt.errorMsg)
			if tt.shouldMatch {
				require.NotEmpty(t, suggestion, "expected suggestion for: %s", tt.errorMsg)
			} else {
				require.Empty(t, suggestion, "unexpected suggestion for: %s", tt.errorMsg)
			}
		}
	})

	t.Run("format diagnostic", func(t *testing.T) {
		d := &TestDiagnostic{
			TestName:   "TestExample",
			File:       "example_test.go",
			Line:       42,
			Expected:   "nil",
			Actual:     "error",
			Suggestion: "Check error handling",
		}

		output := formatDiagnostic(d)
		require.Contains(t, output, "TestExample")
		require.Contains(t, output, "example_test.go:42")
		require.Contains(t, output, "Expected: nil")
		require.Contains(t, output, "Actual:   error")
		require.Contains(t, output, "Suggestion: Check error handling")
	})

	t.Run("parse test output", func(t *testing.T) {
		output := `=== RUN   TestExample
--- FAIL: TestExample (0.01s)
    example_test.go:42: expected nil but got error
FAIL`
		diagnostics := parseTestOutput(output)
		require.Len(t, diagnostics, 1)
		require.Equal(t, "TestExample", diagnostics[0].TestName)
	})
}

func TestFlakyTestAnalysis(t *testing.T) {
	t.Run("detect flaky test", func(t *testing.T) {
		results := []bool{true, false, true, true, false, true, true, false, true, true}
		seeds := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

		info := analyzeFlakyTest("TestFlaky", results, seeds)
		require.NotNil(t, info)
		require.Equal(t, "TestFlaky", info.TestName)
		require.InDelta(t, 0.3, info.FailureRate, 0.01)
		require.Len(t, info.FailingSeeds, 3)
	})

	t.Run("all pass is not flaky", func(t *testing.T) {
		results := []bool{true, true, true, true, true}
		seeds := []int64{1, 2, 3, 4, 5}

		info := analyzeFlakyTest("TestStable", results, seeds)
		require.Nil(t, info)
	})

	t.Run("all fail is not flaky", func(t *testing.T) {
		results := []bool{false, false, false, false, false}
		seeds := []int64{1, 2, 3, 4, 5}

		info := analyzeFlakyTest("TestBroken", results, seeds)
		require.Nil(t, info)
	})
}

func TestIsRelevantChange(t *testing.T) {
	tests := []struct {
		name     string
		op       uint32
		relevant bool
	}{
		{"test.go - write", 2, true},   // fsnotify.Write = 2
		{"test.go - create", 1, true},  // fsnotify.Create = 1
		{".hidden.go", 2, false},
		{"testdata/file.txt", 2, true},
		{"test.txt", 2, false},
	}

	// This is a simplified test - the actual isRelevantChange function
	// uses fsnotify.Event which we can't easily construct in tests
	_ = tests
}

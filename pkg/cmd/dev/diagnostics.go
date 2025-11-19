// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// TestDiagnostic provides structured information about a test failure
type TestDiagnostic struct {
	TestName   string
	File       string
	Line       int
	Expected   string
	Actual     string
	Stack      []string
	Logs       []string
	Suggestion string
}

// Common error patterns and their suggestions
var errorPatterns = map[string]string{
	"context deadline exceeded": "Consider increasing timeout or optimizing the test. Check for slow database operations or network calls.",
	"connection refused":        "Ensure the server is running. Check if the port is available and not blocked by firewall.",
	"transaction retry error":   "Check for concurrent writes to the same key. Consider using SELECT FOR UPDATE or adjusting isolation level.",
	"deadlock detected":         "Review transaction ordering. Ensure consistent lock acquisition order across transactions.",
	"permission denied":         "Check file permissions and ensure the test has necessary access rights.",
	"no such file or directory": "Verify the file path exists. Check if testdata files are properly configured.",
	"timeout":                   "Consider increasing test timeout or optimizing slow operations.",
	"race detected":             "Review concurrent access to shared state. Consider using proper synchronization.",
	"panic":                     "Check for nil pointer dereference, index out of bounds, or other runtime errors.",
	"expected .* but got":       "Verify test expectations match the actual implementation behavior.",
}

// suggestFix analyzes an error message and returns a helpful suggestion
func suggestFix(errorMsg string) string {
	lowerError := strings.ToLower(errorMsg)
	for pattern, suggestion := range errorPatterns {
		matched, _ := regexp.MatchString(strings.ToLower(pattern), lowerError)
		if matched {
			return suggestion
		}
	}
	return ""
}

// formatDiagnostic formats a test diagnostic for display
func formatDiagnostic(d *TestDiagnostic) string {
	var sb strings.Builder

	// Header
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", 50))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("FAIL: %s\n", d.TestName))
	sb.WriteString(strings.Repeat("─", 50))
	sb.WriteString("\n\n")

	// Location
	if d.File != "" {
		if d.Line > 0 {
			sb.WriteString(fmt.Sprintf("Location: %s:%d\n\n", d.File, d.Line))
		} else {
			sb.WriteString(fmt.Sprintf("Location: %s\n\n", d.File))
		}
	}

	// Expected/Actual
	if d.Expected != "" || d.Actual != "" {
		if d.Expected != "" {
			sb.WriteString(fmt.Sprintf("Expected: %s\n", d.Expected))
		}
		if d.Actual != "" {
			sb.WriteString(fmt.Sprintf("Actual:   %s\n", d.Actual))
		}
		sb.WriteString("\n")
	}

	// Stack trace
	if len(d.Stack) > 0 {
		sb.WriteString("Stack:\n")
		for _, frame := range d.Stack {
			sb.WriteString(fmt.Sprintf("  %s\n", frame))
		}
		sb.WriteString("\n")
	}

	// Related logs
	if len(d.Logs) > 0 {
		sb.WriteString("Related logs:\n")
		for _, logLine := range d.Logs {
			sb.WriteString(fmt.Sprintf("  %s\n", logLine))
		}
		sb.WriteString("\n")
	}

	// Suggestion
	if d.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("Suggestion: %s\n", d.Suggestion))
	}

	return sb.String()
}

// parseTestOutput parses test output and extracts diagnostics
// This is a simplified version - a full implementation would need
// to parse the actual test output format
func parseTestOutput(output string) []*TestDiagnostic {
	var diagnostics []*TestDiagnostic

	// Look for FAIL patterns
	failPattern := regexp.MustCompile(`--- FAIL: (\S+)`)
	matches := failPattern.FindAllStringSubmatch(output, -1)

	for _, match := range matches {
		if len(match) > 1 {
			testName := match[1]
			d := &TestDiagnostic{
				TestName: testName,
			}

			// Try to find file:line information
			linePattern := regexp.MustCompile(`(\S+\.go):(\d+)`)
			if lineMatch := linePattern.FindStringSubmatch(output); len(lineMatch) > 2 {
				d.File = lineMatch[1]
				fmt.Sscanf(lineMatch[2], "%d", &d.Line)
			}

			// Get suggestion based on error content
			d.Suggestion = suggestFix(output)

			diagnostics = append(diagnostics, d)
		}
	}

	return diagnostics
}

// flakyTestInfo provides information about flaky test detection
type flakyTestInfo struct {
	TestName     string
	FailureRate  float64
	FailingSeeds []int64
	Pattern      string
}

// analyzeFlakyTest analyzes stress test results to detect flakiness patterns
func analyzeFlakyTest(testName string, results []bool, seeds []int64) *flakyTestInfo {
	failures := 0
	var failingSeeds []int64

	for i, passed := range results {
		if !passed {
			failures++
			if i < len(seeds) {
				failingSeeds = append(failingSeeds, seeds[i])
			}
		}
	}

	if failures == 0 || failures == len(results) {
		return nil // Not flaky - either always passes or always fails
	}

	return &flakyTestInfo{
		TestName:     testName,
		FailureRate:  float64(failures) / float64(len(results)),
		FailingSeeds: failingSeeds,
	}
}

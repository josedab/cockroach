// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package main

import (
	"fmt"
	"time"
)

const (
	profileFlag = "profile"
)

// TestProfile defines a pre-configured set of testing options
type TestProfile struct {
	Name        string
	Description string
	Timeout     time.Duration
	Race        bool
	Short       bool
	Stress      int // Number of stress iterations (0 = disabled)
	Verbose     bool
	IgnoreCache bool
}

// predefinedProfiles contains the built-in test profiles
var predefinedProfiles = map[string]*TestProfile{
	"fast": {
		Name:        "fast",
		Description: "Fast iteration - skip slow tests",
		Timeout:     1 * time.Minute,
		Race:        false,
		Short:       true,
		Stress:      0,
		Verbose:     false,
		IgnoreCache: false,
	},
	"thorough": {
		Name:        "thorough",
		Description: "Thorough testing - include race detection",
		Timeout:     10 * time.Minute,
		Race:        true,
		Short:       false,
		Stress:      0,
		Verbose:     true,
		IgnoreCache: false,
	},
	"stress": {
		Name:        "stress",
		Description: "Stress testing - run tests multiple times",
		Timeout:     5 * time.Minute,
		Race:        false,
		Short:       false,
		Stress:      100,
		Verbose:     false,
		IgnoreCache: true,
	},
	"ci": {
		Name:        "ci",
		Description: "CI simulation - settings similar to CI",
		Timeout:     5 * time.Minute,
		Race:        true,
		Short:       false,
		Stress:      0,
		Verbose:     true,
		IgnoreCache: true,
	},
}

// getProfile returns the test profile by name, or an error if not found
func getProfile(name string) (*TestProfile, error) {
	profile, ok := predefinedProfiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q. Available profiles: fast, thorough, stress, ci", name)
	}
	return profile, nil
}

// listProfiles returns a description of all available profiles
func listProfiles() string {
	result := "Available test profiles:\n"
	for name, profile := range predefinedProfiles {
		result += fmt.Sprintf("  %s: %s\n", name, profile.Description)
	}
	return result
}

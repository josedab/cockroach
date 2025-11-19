// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// Package autoscale implements predictive auto-scaling based on workload patterns.
package autoscale

import (
	"context"
	"fmt"
	"time"
)

// WorkloadMetrics contains the collected workload indicators used for
// pattern detection and load prediction.
type WorkloadMetrics struct {
	// Timestamp when metrics were collected.
	Timestamp time.Time
	// QueriesPerSec is the number of queries executed per second.
	QueriesPerSec float64
	// ActiveConns is the number of active client connections.
	ActiveConns int
	// CPUUtilization as a percentage (0-100).
	CPUUtilization float64
	// MemoryUsage as a percentage (0-100).
	MemoryUsage float64
	// DiskIO in bytes per second.
	DiskIO float64
	// P99Latency is the 99th percentile query latency.
	P99Latency time.Duration
	// NodeID identifies which node these metrics are from.
	NodeID int32
}

// PatternType represents the type of workload pattern detected.
type PatternType int

const (
	// PatternDaily represents patterns that repeat daily.
	PatternDaily PatternType = iota
	// PatternWeekly represents patterns that repeat weekly.
	PatternWeekly
	// PatternSeasonal represents seasonal patterns (monthly, yearly).
	PatternSeasonal
	// PatternCustom represents user-defined patterns.
	PatternCustom
)

// String returns a human-readable name for the pattern type.
func (pt PatternType) String() string {
	switch pt {
	case PatternDaily:
		return "daily"
	case PatternWeekly:
		return "weekly"
	case PatternSeasonal:
		return "seasonal"
	case PatternCustom:
		return "custom"
	default:
		return fmt.Sprintf("unknown(%d)", pt)
	}
}

// TimeRange represents a time range within a pattern.
type TimeRange struct {
	// Start hour (0-23).
	StartHour int
	// End hour (0-23).
	EndHour int
	// DayOfWeek (0=Sunday, 6=Saturday), -1 for all days.
	DayOfWeek int
}

// Contains checks if a given time falls within this range.
func (tr TimeRange) Contains(t time.Time) bool {
	hour := t.Hour()
	if tr.DayOfWeek >= 0 && int(t.Weekday()) != tr.DayOfWeek {
		return false
	}
	if tr.StartHour <= tr.EndHour {
		return hour >= tr.StartHour && hour < tr.EndHour
	}
	// Handle wrap-around (e.g., 22:00 - 06:00)
	return hour >= tr.StartHour || hour < tr.EndHour
}

// Pattern represents a detected workload pattern.
type Pattern struct {
	// Type is the pattern type (daily, weekly, seasonal).
	Type PatternType
	// PeakTimes are the time ranges when load is expected to be high.
	PeakTimes []TimeRange
	// ScaleFactor is the multiplier to apply during peak times.
	ScaleFactor float64
	// Confidence is the confidence level of this pattern (0-1).
	Confidence float64
	// Description is a human-readable description of the pattern.
	Description string
}

// Applies checks if this pattern applies to the given time.
func (p Pattern) Applies(t time.Time) bool {
	for _, tr := range p.PeakTimes {
		if tr.Contains(t) {
			return true
		}
	}
	return false
}

// EventPattern represents non-periodic patterns triggered by events.
type EventPattern struct {
	// Trigger is the event identifier (e.g., "holiday", "promotion").
	Trigger string
	// Impact is the load multiplier when this event occurs.
	Impact float64
	// LeadTime is how long before the event to start scaling.
	LeadTime time.Duration
	// StartTime is when the event starts.
	StartTime time.Time
	// EndTime is when the event ends.
	EndTime time.Time
}

// ScaleAction represents the type of scaling action to take.
type ScaleAction int

const (
	// ScaleNoOp indicates no scaling action is needed.
	ScaleNoOp ScaleAction = iota
	// ScaleUp indicates nodes should be added.
	ScaleUp
	// ScaleDown indicates nodes should be removed.
	ScaleDown
)

// String returns a human-readable name for the scale action.
func (sa ScaleAction) String() string {
	switch sa {
	case ScaleNoOp:
		return "no-op"
	case ScaleUp:
		return "scale-up"
	case ScaleDown:
		return "scale-down"
	default:
		return fmt.Sprintf("unknown(%d)", sa)
	}
}

// ScalingDecision represents a decision made by the scaling engine.
type ScalingDecision struct {
	// Action is the scaling action to take.
	Action ScaleAction
	// TargetNodes is the target number of nodes after scaling.
	TargetNodes int
	// Reason is a human-readable explanation for the decision.
	Reason string
	// Confidence is the confidence level of this decision (0-1).
	Confidence float64
	// PredictedLoad is the predicted load that triggered this decision.
	PredictedLoad float64
	// Timestamp when this decision was made.
	Timestamp time.Time
}

// ScalingConfig contains the configuration for the scaling engine.
type ScalingConfig struct {
	// MinNodes is the minimum number of nodes in the cluster.
	MinNodes int
	// MaxNodes is the maximum number of nodes in the cluster.
	MaxNodes int
	// ScaleUpThreshold is the load ratio that triggers scale up.
	ScaleUpThreshold float64
	// ScaleDownThreshold is the load ratio that triggers scale down.
	ScaleDownThreshold float64
	// LeadTime is how long before predicted peak to start scaling.
	LeadTime time.Duration
	// Cooldown is the minimum time between scaling operations.
	Cooldown time.Duration
	// ConfidenceThreshold is the minimum confidence for scaling.
	ConfidenceThreshold float64
	// SafetyMargin is the multiplier applied to predictions for safety.
	SafetyMargin float64
}

// DefaultScalingConfig returns a default scaling configuration.
func DefaultScalingConfig() ScalingConfig {
	return ScalingConfig{
		MinNodes:            3,
		MaxNodes:            10,
		ScaleUpThreshold:    0.8,
		ScaleDownThreshold:  0.3,
		LeadTime:            15 * time.Minute,
		Cooldown:            10 * time.Minute,
		ConfidenceThreshold: 0.7,
		SafetyMargin:        1.2,
	}
}

// Prediction represents a load prediction for a future time.
type Prediction struct {
	// Time is when this prediction is for.
	Time time.Time
	// Load is the predicted load value.
	Load float64
	// Confidence is the confidence level of this prediction (0-1).
	Confidence float64
	// RecommendedAction is the suggested scaling action.
	RecommendedAction ScaleAction
	// RecommendedNodes is the suggested number of nodes.
	RecommendedNodes int
}

// ScalingHistoryEntry represents a historical scaling event.
type ScalingHistoryEntry struct {
	// Timestamp when the scaling occurred.
	Timestamp time.Time
	// Action that was taken.
	Action ScaleAction
	// FromNodes is the number of nodes before scaling.
	FromNodes int
	// ToNodes is the number of nodes after scaling.
	ToNodes int
	// Reason for the scaling action.
	Reason string
	// Confidence of the decision.
	Confidence float64
	// Success indicates if the scaling completed successfully.
	Success bool
	// Error message if scaling failed.
	Error string
}

// CloudScaler is the interface for cloud provider scaling operations.
type CloudScaler interface {
	// ScaleUp adds nodes to the cluster.
	ScaleUp(ctx context.Context, count int) error
	// ScaleDown removes specific nodes from the cluster.
	ScaleDown(ctx context.Context, nodeIDs []int32) error
	// GetCurrentNodeCount returns the current number of nodes.
	GetCurrentNodeCount(ctx context.Context) (int, error)
}

// MetricsCollector is the interface for collecting workload metrics.
type MetricsCollector interface {
	// Collect gathers current workload metrics from the cluster.
	Collect(ctx context.Context) ([]WorkloadMetrics, error)
	// GetHistorical returns historical metrics for the given time range.
	GetHistorical(ctx context.Context, start, end time.Time) ([]WorkloadMetrics, error)
}

// PatternDetector is the interface for detecting workload patterns.
type PatternDetector interface {
	// Detect analyzes metrics to find recurring patterns.
	Detect(ctx context.Context, metrics []WorkloadMetrics) ([]Pattern, error)
}

// LoadPredictor is the interface for predicting future load.
type LoadPredictor interface {
	// Predict estimates load for a future time.
	Predict(ctx context.Context, t time.Time, history []WorkloadMetrics, patterns []Pattern) (Prediction, error)
	// PredictRange estimates load for a range of future times.
	PredictRange(ctx context.Context, start, end time.Time, interval time.Duration, history []WorkloadMetrics, patterns []Pattern) ([]Prediction, error)
}

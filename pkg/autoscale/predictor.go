// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package autoscale

import (
	"context"
	"sort"
	"time"
)

// DefaultPredictor implements LoadPredictor using historical averages and patterns.
type DefaultPredictor struct {
	// SafetyMargin is the multiplier applied to predictions.
	SafetyMargin float64
	// MinHistoryDays is the minimum days of history required.
	MinHistoryDays int
}

// NewPredictor creates a new DefaultPredictor with default settings.
func NewPredictor() *DefaultPredictor {
	return &DefaultPredictor{
		SafetyMargin:   1.2,
		MinHistoryDays: 7,
	}
}

// Predict estimates load for a future time.
func (p *DefaultPredictor) Predict(
	ctx context.Context,
	t time.Time,
	history []WorkloadMetrics,
	patterns []Pattern,
) (Prediction, error) {
	if len(history) == 0 {
		return Prediction{
			Time:       t,
			Load:       0,
			Confidence: 0,
		}, nil
	}

	// Calculate base prediction from historical average
	base := p.historicalAverage(history, t)

	// Apply pattern multipliers
	for _, pattern := range patterns {
		if pattern.Applies(t) {
			base *= pattern.ScaleFactor
		}
	}

	// Apply safety margin
	predicted := base * p.SafetyMargin

	// Calculate confidence based on data quality
	confidence := p.calculateConfidence(history, patterns)

	return Prediction{
		Time:       t,
		Load:       predicted,
		Confidence: confidence,
	}, nil
}

// PredictRange estimates load for a range of future times.
func (p *DefaultPredictor) PredictRange(
	ctx context.Context,
	start, end time.Time,
	interval time.Duration,
	history []WorkloadMetrics,
	patterns []Pattern,
) ([]Prediction, error) {
	var predictions []Prediction

	for t := start; t.Before(end) || t.Equal(end); t = t.Add(interval) {
		pred, err := p.Predict(ctx, t, history, patterns)
		if err != nil {
			return nil, err
		}
		predictions = append(predictions, pred)
	}

	return predictions, nil
}

// historicalAverage calculates the average load for similar times in history.
func (p *DefaultPredictor) historicalAverage(history []WorkloadMetrics, t time.Time) float64 {
	targetHour := t.Hour()
	targetWeekday := int(t.Weekday())

	var similarLoads []float64

	for _, m := range history {
		// Match by hour and day of week
		if m.Timestamp.Hour() == targetHour && int(m.Timestamp.Weekday()) == targetWeekday {
			load := p.calculateLoad(m)
			similarLoads = append(similarLoads, load)
		}
	}

	if len(similarLoads) == 0 {
		// Fall back to same hour, any day
		for _, m := range history {
			if m.Timestamp.Hour() == targetHour {
				load := p.calculateLoad(m)
				similarLoads = append(similarLoads, load)
			}
		}
	}

	if len(similarLoads) == 0 {
		// Fall back to overall average
		for _, m := range history {
			load := p.calculateLoad(m)
			similarLoads = append(similarLoads, load)
		}
	}

	return mean(similarLoads)
}

// calculateLoad computes a composite load score from metrics.
func (p *DefaultPredictor) calculateLoad(m WorkloadMetrics) float64 {
	// Weighted combination of metrics
	qpsWeight := 0.5
	cpuWeight := 0.3
	memWeight := 0.2

	// Normalize QPS
	normalizedQPS := m.QueriesPerSec / 10000.0
	if normalizedQPS > 1.0 {
		normalizedQPS = 1.0
	}

	normalizedCPU := m.CPUUtilization / 100.0
	normalizedMem := m.MemoryUsage / 100.0

	return qpsWeight*normalizedQPS + cpuWeight*normalizedCPU + memWeight*normalizedMem
}

// calculateConfidence estimates prediction confidence based on data quality.
func (p *DefaultPredictor) calculateConfidence(history []WorkloadMetrics, patterns []Pattern) float64 {
	if len(history) == 0 {
		return 0
	}

	// Base confidence from data quantity
	// More data = higher confidence
	dataConfidence := 0.0
	if len(history) >= 168 { // 1 week of hourly data
		dataConfidence = 0.5
	} else if len(history) >= 24 { // 1 day
		dataConfidence = 0.3
	} else {
		dataConfidence = 0.1
	}

	// Add confidence from detected patterns
	patternConfidence := 0.0
	for _, p := range patterns {
		patternConfidence += p.Confidence * 0.25
	}
	if patternConfidence > 0.5 {
		patternConfidence = 0.5
	}

	return dataConfidence + patternConfidence
}

// ScalingEngine makes scaling decisions based on predictions.
type ScalingEngine struct {
	config ScalingConfig
}

// NewScalingEngine creates a new ScalingEngine with the given configuration.
func NewScalingEngine(config ScalingConfig) *ScalingEngine {
	return &ScalingEngine{
		config: config,
	}
}

// Decide determines what scaling action to take based on predicted vs current load.
func (e *ScalingEngine) Decide(
	predicted float64, current float64, currentNodes int,
) ScalingDecision {
	if current == 0 {
		return ScalingDecision{
			Action:     ScaleNoOp,
			Reason:     "No current load data available",
			Confidence: 0,
			Timestamp:  time.Now(),
		}
	}

	ratio := predicted / current

	if ratio > 1.0/e.config.ScaleUpThreshold {
		// Need to scale up
		targetNodes := e.calculateTargetNodes(predicted, currentNodes)
		if targetNodes > currentNodes {
			return ScalingDecision{
				Action:        ScaleUp,
				TargetNodes:   targetNodes,
				Reason:        "Predicted load exceeds current capacity",
				Confidence:    0.85,
				PredictedLoad: predicted,
				Timestamp:     time.Now(),
			}
		}
	}

	if ratio < e.config.ScaleDownThreshold {
		// Can scale down
		targetNodes := e.calculateTargetNodes(predicted, currentNodes)
		if targetNodes < currentNodes && targetNodes >= e.config.MinNodes {
			return ScalingDecision{
				Action:        ScaleDown,
				TargetNodes:   targetNodes,
				Reason:        "Predicted load is significantly below current capacity",
				Confidence:    0.75,
				PredictedLoad: predicted,
				Timestamp:     time.Now(),
			}
		}
	}

	return ScalingDecision{
		Action:        ScaleNoOp,
		TargetNodes:   currentNodes,
		Reason:        "Current capacity is appropriate for predicted load",
		Confidence:    0.9,
		PredictedLoad: predicted,
		Timestamp:     time.Now(),
	}
}

// calculateTargetNodes determines how many nodes are needed for the predicted load.
func (e *ScalingEngine) calculateTargetNodes(predicted float64, currentNodes int) int {
	// Simple linear scaling
	// Assume each node can handle a normalized load of 0.8
	nodesNeeded := int(predicted/0.8) + 1

	// Apply constraints
	if nodesNeeded < e.config.MinNodes {
		nodesNeeded = e.config.MinNodes
	}
	if nodesNeeded > e.config.MaxNodes {
		nodesNeeded = e.config.MaxNodes
	}

	return nodesNeeded
}

// PredictionService orchestrates metrics collection, pattern detection, and prediction.
type PredictionService struct {
	detector  PatternDetector
	predictor LoadPredictor
	engine    *ScalingEngine
}

// NewPredictionService creates a new PredictionService.
func NewPredictionService(config ScalingConfig) *PredictionService {
	predictor := NewPredictor()
	predictor.SafetyMargin = config.SafetyMargin

	return &PredictionService{
		detector:  NewPatternDetector(),
		predictor: predictor,
		engine:    NewScalingEngine(config),
	}
}

// GeneratePredictions creates predictions for the specified time range.
func (s *PredictionService) GeneratePredictions(
	ctx context.Context,
	history []WorkloadMetrics,
	start, end time.Time,
	interval time.Duration,
	currentNodes int,
) ([]Prediction, error) {
	// Detect patterns
	patterns, err := s.detector.Detect(ctx, history)
	if err != nil {
		return nil, err
	}

	// Generate predictions
	predictions, err := s.predictor.PredictRange(ctx, start, end, interval, history, patterns)
	if err != nil {
		return nil, err
	}

	// Calculate current load
	currentLoad := s.calculateCurrentLoad(history)

	// Add scaling recommendations to predictions
	for i := range predictions {
		decision := s.engine.Decide(predictions[i].Load, currentLoad, currentNodes)
		predictions[i].RecommendedAction = decision.Action
		predictions[i].RecommendedNodes = decision.TargetNodes
	}

	return predictions, nil
}

// calculateCurrentLoad gets the most recent load value.
func (s *PredictionService) calculateCurrentLoad(history []WorkloadMetrics) float64 {
	if len(history) == 0 {
		return 0
	}

	// Find the most recent metric
	sorted := make([]WorkloadMetrics, len(history))
	copy(sorted, history)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})

	// Use average of last few readings
	count := 5
	if len(sorted) < count {
		count = len(sorted)
	}

	predictor := NewPredictor()
	var loads []float64
	for i := 0; i < count; i++ {
		loads = append(loads, predictor.calculateLoad(sorted[i]))
	}

	return mean(loads)
}

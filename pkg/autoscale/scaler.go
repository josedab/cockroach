// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package autoscale

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockScaler is a mock implementation of CloudScaler for testing.
type MockScaler struct {
	mu         sync.Mutex
	nodeCount  int
	scaleUpFn  func(count int) error
	scaleDownFn func(nodeIDs []int32) error
}

// NewMockScaler creates a new MockScaler with the specified initial node count.
func NewMockScaler(initialNodes int) *MockScaler {
	return &MockScaler{
		nodeCount: initialNodes,
	}
}

// ScaleUp adds nodes to the cluster.
func (s *MockScaler) ScaleUp(ctx context.Context, count int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.scaleUpFn != nil {
		if err := s.scaleUpFn(count); err != nil {
			return err
		}
	}

	s.nodeCount += count
	return nil
}

// ScaleDown removes specific nodes from the cluster.
func (s *MockScaler) ScaleDown(ctx context.Context, nodeIDs []int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.scaleDownFn != nil {
		if err := s.scaleDownFn(nodeIDs); err != nil {
			return err
		}
	}

	s.nodeCount -= len(nodeIDs)
	if s.nodeCount < 1 {
		s.nodeCount = 1
	}
	return nil
}

// GetCurrentNodeCount returns the current number of nodes.
func (s *MockScaler) GetCurrentNodeCount(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nodeCount, nil
}

// SetScaleUpFn sets a custom function for scale up operations.
func (s *MockScaler) SetScaleUpFn(fn func(count int) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scaleUpFn = fn
}

// SetScaleDownFn sets a custom function for scale down operations.
func (s *MockScaler) SetScaleDownFn(fn func(nodeIDs []int32) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scaleDownFn = fn
}

// Controller coordinates the autoscaling process.
type Controller struct {
	config          ScalingConfig
	scaler          CloudScaler
	collector       MetricsCollector
	service         *PredictionService

	mu              sync.Mutex
	lastScaleTime   time.Time
	history         []ScalingHistoryEntry
	running         bool
	stopCh          chan struct{}
}

// NewController creates a new autoscaling Controller.
func NewController(
	config ScalingConfig,
	scaler CloudScaler,
	collector MetricsCollector,
) *Controller {
	return &Controller{
		config:    config,
		scaler:    scaler,
		collector: collector,
		service:   NewPredictionService(config),
		history:   make([]ScalingHistoryEntry, 0),
		stopCh:    make(chan struct{}),
	}
}

// Start begins the autoscaling control loop.
func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("controller already running")
	}
	c.running = true
	c.mu.Unlock()

	go c.run(ctx)
	return nil
}

// Stop halts the autoscaling control loop.
func (c *Controller) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	c.mu.Unlock()

	close(c.stopCh)
}

// run is the main control loop.
func (c *Controller) run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			if err := c.evaluate(ctx); err != nil {
				// Log error but continue
				continue
			}
		}
	}
}

// evaluate performs one iteration of the autoscaling evaluation.
func (c *Controller) evaluate(ctx context.Context) error {
	// Check cooldown
	c.mu.Lock()
	if time.Since(c.lastScaleTime) < c.config.Cooldown {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// Collect metrics
	end := time.Now()
	start := end.Add(-30 * 24 * time.Hour) // Last 30 days
	history, err := c.collector.GetHistorical(ctx, start, end)
	if err != nil {
		return err
	}

	// Get current node count
	currentNodes, err := c.scaler.GetCurrentNodeCount(ctx)
	if err != nil {
		return err
	}

	// Generate predictions
	predStart := time.Now()
	predEnd := predStart.Add(c.config.LeadTime * 2)
	predictions, err := c.service.GeneratePredictions(
		ctx, history, predStart, predEnd, 5*time.Minute, currentNodes,
	)
	if err != nil {
		return err
	}

	// Find first prediction that requires action
	for _, pred := range predictions {
		if pred.Confidence < c.config.ConfidenceThreshold {
			continue
		}

		if pred.RecommendedAction == ScaleNoOp {
			continue
		}

		// Check if action is needed within lead time
		if pred.Time.After(time.Now().Add(c.config.LeadTime)) {
			continue
		}

		// Execute scaling
		if err := c.executeScaling(ctx, pred, currentNodes); err != nil {
			return err
		}
		break
	}

	return nil
}

// executeScaling performs the scaling operation.
func (c *Controller) executeScaling(ctx context.Context, pred Prediction, currentNodes int) error {
	entry := ScalingHistoryEntry{
		Timestamp:  time.Now(),
		Action:     pred.RecommendedAction,
		FromNodes:  currentNodes,
		ToNodes:    pred.RecommendedNodes,
		Reason:     fmt.Sprintf("Predicted load: %.2f at %s", pred.Load, pred.Time.Format(time.RFC3339)),
		Confidence: pred.Confidence,
	}

	var err error
	switch pred.RecommendedAction {
	case ScaleUp:
		nodesToAdd := pred.RecommendedNodes - currentNodes
		err = c.scaler.ScaleUp(ctx, nodesToAdd)
	case ScaleDown:
		nodesToRemove := currentNodes - pred.RecommendedNodes
		// In a real implementation, we'd select specific nodes to remove
		nodeIDs := make([]int32, nodesToRemove)
		for i := range nodeIDs {
			nodeIDs[i] = int32(currentNodes - i)
		}
		err = c.scaler.ScaleDown(ctx, nodeIDs)
	}

	if err != nil {
		entry.Success = false
		entry.Error = err.Error()
	} else {
		entry.Success = true
		c.mu.Lock()
		c.lastScaleTime = time.Now()
		c.mu.Unlock()
	}

	c.addHistory(entry)
	return err
}

// addHistory adds an entry to the scaling history.
func (c *Controller) addHistory(entry ScalingHistoryEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.history = append(c.history, entry)

	// Keep only last 1000 entries
	if len(c.history) > 1000 {
		c.history = c.history[len(c.history)-1000:]
	}
}

// GetHistory returns the scaling history.
func (c *Controller) GetHistory() []ScalingHistoryEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]ScalingHistoryEntry, len(c.history))
	copy(result, c.history)
	return result
}

// GetPredictions generates and returns current predictions.
func (c *Controller) GetPredictions(ctx context.Context) ([]Prediction, error) {
	// Collect metrics
	end := time.Now()
	start := end.Add(-30 * 24 * time.Hour)
	history, err := c.collector.GetHistorical(ctx, start, end)
	if err != nil {
		return nil, err
	}

	// Get current node count
	currentNodes, err := c.scaler.GetCurrentNodeCount(ctx)
	if err != nil {
		return nil, err
	}

	// Generate predictions for next 24 hours
	predStart := time.Now()
	predEnd := predStart.Add(24 * time.Hour)
	return c.service.GeneratePredictions(
		ctx, history, predStart, predEnd, time.Hour, currentNodes,
	)
}

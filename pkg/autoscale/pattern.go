// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package autoscale

import (
	"context"
	"math"
	"sort"
	"time"
)

// DefaultPatternDetector implements PatternDetector using statistical analysis.
type DefaultPatternDetector struct {
	// MinDataPoints is the minimum number of data points required for pattern detection.
	MinDataPoints int
	// MinConfidence is the minimum confidence level to report a pattern.
	MinConfidence float64
}

// NewPatternDetector creates a new DefaultPatternDetector with default settings.
func NewPatternDetector() *DefaultPatternDetector {
	return &DefaultPatternDetector{
		MinDataPoints: 168, // At least 1 week of hourly data
		MinConfidence: 0.5,
	}
}

// Detect analyzes metrics to find recurring patterns.
func (d *DefaultPatternDetector) Detect(
	ctx context.Context, metrics []WorkloadMetrics,
) ([]Pattern, error) {
	if len(metrics) < d.MinDataPoints {
		return nil, nil
	}

	// Sort metrics by timestamp
	sorted := make([]WorkloadMetrics, len(metrics))
	copy(sorted, metrics)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	var patterns []Pattern

	// Detect daily patterns
	if dailyPattern := d.detectDailyPattern(sorted); dailyPattern != nil {
		patterns = append(patterns, *dailyPattern)
	}

	// Detect weekly patterns
	if weeklyPattern := d.detectWeeklyPattern(sorted); weeklyPattern != nil {
		patterns = append(patterns, *weeklyPattern)
	}

	return patterns, nil
}

// detectDailyPattern looks for patterns that repeat every 24 hours.
func (d *DefaultPatternDetector) detectDailyPattern(metrics []WorkloadMetrics) *Pattern {
	// Group metrics by hour of day
	hourlyLoads := make(map[int][]float64)
	for _, m := range metrics {
		hour := m.Timestamp.Hour()
		load := d.calculateLoad(m)
		hourlyLoads[hour] = append(hourlyLoads[hour], load)
	}

	// Calculate average load per hour
	hourlyAvg := make(map[int]float64)
	for hour, loads := range hourlyLoads {
		hourlyAvg[hour] = mean(loads)
	}

	// Find overall average
	var allLoads []float64
	for _, avg := range hourlyAvg {
		allLoads = append(allLoads, avg)
	}
	overallAvg := mean(allLoads)

	// Find peak hours (above average)
	var peakHours []int
	for hour, avg := range hourlyAvg {
		if avg > overallAvg*1.2 { // 20% above average
			peakHours = append(peakHours, hour)
		}
	}

	if len(peakHours) == 0 {
		return nil
	}

	// Sort peak hours and create time ranges
	sort.Ints(peakHours)
	ranges := d.createTimeRanges(peakHours, -1) // -1 means all days

	// Calculate confidence based on consistency
	confidence := d.calculateDailyConfidence(hourlyLoads)

	if confidence < d.MinConfidence {
		return nil
	}

	// Calculate scale factor
	peakLoad := 0.0
	for _, hour := range peakHours {
		if hourlyAvg[hour] > peakLoad {
			peakLoad = hourlyAvg[hour]
		}
	}
	scaleFactor := peakLoad / overallAvg

	return &Pattern{
		Type:        PatternDaily,
		PeakTimes:   ranges,
		ScaleFactor: scaleFactor,
		Confidence:  confidence,
		Description: "Daily workload pattern detected",
	}
}

// detectWeeklyPattern looks for patterns that vary by day of week.
func (d *DefaultPatternDetector) detectWeeklyPattern(metrics []WorkloadMetrics) *Pattern {
	// Group metrics by day of week
	dailyLoads := make(map[int][]float64)
	for _, m := range metrics {
		day := int(m.Timestamp.Weekday())
		load := d.calculateLoad(m)
		dailyLoads[day] = append(dailyLoads[day], load)
	}

	// Calculate average load per day
	dailyAvg := make(map[int]float64)
	for day, loads := range dailyLoads {
		dailyAvg[day] = mean(loads)
	}

	// Find overall average
	var allLoads []float64
	for _, avg := range dailyAvg {
		allLoads = append(allLoads, avg)
	}
	overallAvg := mean(allLoads)

	// Find peak days (above average)
	var peakDays []int
	maxLoad := 0.0
	for day, avg := range dailyAvg {
		if avg > overallAvg*1.2 { // 20% above average
			peakDays = append(peakDays, day)
		}
		if avg > maxLoad {
			maxLoad = avg
		}
	}

	if len(peakDays) == 0 {
		return nil
	}

	// Calculate confidence
	confidence := d.calculateWeeklyConfidence(dailyLoads)

	if confidence < d.MinConfidence {
		return nil
	}

	// Create time ranges for peak days (all hours)
	var ranges []TimeRange
	for _, day := range peakDays {
		ranges = append(ranges, TimeRange{
			StartHour: 0,
			EndHour:   24,
			DayOfWeek: day,
		})
	}

	scaleFactor := maxLoad / overallAvg

	return &Pattern{
		Type:        PatternWeekly,
		PeakTimes:   ranges,
		ScaleFactor: scaleFactor,
		Confidence:  confidence,
		Description: "Weekly workload pattern detected",
	}
}

// calculateLoad computes a composite load score from metrics.
func (d *DefaultPatternDetector) calculateLoad(m WorkloadMetrics) float64 {
	// Weighted combination of metrics
	// QPS is the primary indicator, with CPU and memory as secondary
	qpsWeight := 0.5
	cpuWeight := 0.3
	memWeight := 0.2

	// Normalize QPS (assume 10000 QPS is high)
	normalizedQPS := m.QueriesPerSec / 10000.0
	if normalizedQPS > 1.0 {
		normalizedQPS = 1.0
	}

	// CPU and memory are already percentages
	normalizedCPU := m.CPUUtilization / 100.0
	normalizedMem := m.MemoryUsage / 100.0

	return qpsWeight*normalizedQPS + cpuWeight*normalizedCPU + memWeight*normalizedMem
}

// createTimeRanges groups consecutive hours into time ranges.
func (d *DefaultPatternDetector) createTimeRanges(hours []int, dayOfWeek int) []TimeRange {
	if len(hours) == 0 {
		return nil
	}

	var ranges []TimeRange
	start := hours[0]
	prev := hours[0]

	for i := 1; i < len(hours); i++ {
		if hours[i] != prev+1 {
			// End current range, start new one
			ranges = append(ranges, TimeRange{
				StartHour: start,
				EndHour:   prev + 1,
				DayOfWeek: dayOfWeek,
			})
			start = hours[i]
		}
		prev = hours[i]
	}

	// Add final range
	ranges = append(ranges, TimeRange{
		StartHour: start,
		EndHour:   prev + 1,
		DayOfWeek: dayOfWeek,
	})

	return ranges
}

// calculateDailyConfidence calculates confidence based on hourly load consistency.
func (d *DefaultPatternDetector) calculateDailyConfidence(hourlyLoads map[int][]float64) float64 {
	if len(hourlyLoads) == 0 {
		return 0
	}

	// Calculate coefficient of variation for each hour
	var cvs []float64
	for _, loads := range hourlyLoads {
		if len(loads) < 2 {
			continue
		}
		avg := mean(loads)
		if avg == 0 {
			continue
		}
		stdDev := standardDeviation(loads)
		cv := stdDev / avg
		cvs = append(cvs, cv)
	}

	if len(cvs) == 0 {
		return 0
	}

	// Lower CV means more consistent patterns
	avgCV := mean(cvs)

	// Convert CV to confidence (lower CV = higher confidence)
	// CV of 0.5 or more means low confidence
	confidence := 1.0 - math.Min(avgCV, 1.0)
	return confidence
}

// calculateWeeklyConfidence calculates confidence based on daily load consistency.
func (d *DefaultPatternDetector) calculateWeeklyConfidence(dailyLoads map[int][]float64) float64 {
	if len(dailyLoads) == 0 {
		return 0
	}

	// Calculate coefficient of variation for each day
	var cvs []float64
	for _, loads := range dailyLoads {
		if len(loads) < 2 {
			continue
		}
		avg := mean(loads)
		if avg == 0 {
			continue
		}
		stdDev := standardDeviation(loads)
		cv := stdDev / avg
		cvs = append(cvs, cv)
	}

	if len(cvs) == 0 {
		return 0
	}

	avgCV := mean(cvs)
	confidence := 1.0 - math.Min(avgCV, 1.0)
	return confidence
}

// Helper functions

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func standardDeviation(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	avg := mean(values)
	sumSquares := 0.0
	for _, v := range values {
		diff := v - avg
		sumSquares += diff * diff
	}
	variance := sumSquares / float64(len(values)-1)
	return math.Sqrt(variance)
}

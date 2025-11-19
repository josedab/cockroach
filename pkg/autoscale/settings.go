// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package autoscale

import (
	"time"

	"github.com/cockroachdb/cockroach/pkg/settings"
)

// PredictiveEnabled controls whether predictive auto-scaling is enabled.
var PredictiveEnabled = settings.RegisterBoolSetting(
	settings.SystemOnly,
	"autoscale.predictive.enabled",
	"enables predictive auto-scaling based on workload patterns",
	false,
	settings.WithPublic,
)

// MinNodes sets the minimum number of nodes in the cluster.
var MinNodes = settings.RegisterIntSetting(
	settings.SystemOnly,
	"autoscale.min_nodes",
	"minimum number of nodes to maintain in the cluster",
	3,
	settings.IntInRange(1, 1000),
	settings.WithPublic,
)

// MaxNodes sets the maximum number of nodes in the cluster.
var MaxNodes = settings.RegisterIntSetting(
	settings.SystemOnly,
	"autoscale.max_nodes",
	"maximum number of nodes allowed in the cluster",
	10,
	settings.IntInRange(1, 10000),
	settings.WithPublic,
)

// LeadTime sets how long before predicted peak to start scaling.
var LeadTime = settings.RegisterDurationSetting(
	settings.SystemOnly,
	"autoscale.lead_time",
	"how long before predicted load peak to initiate scaling",
	15*time.Minute,
	settings.DurationInRange(1*time.Minute, 1*time.Hour),
	settings.WithPublic,
)

// Cooldown sets the minimum time between scaling operations.
var Cooldown = settings.RegisterDurationSetting(
	settings.SystemOnly,
	"autoscale.cooldown",
	"minimum time to wait between scaling operations",
	10*time.Minute,
	settings.DurationInRange(1*time.Minute, 1*time.Hour),
	settings.WithPublic,
)

// ConfidenceThreshold sets the minimum confidence required for scaling.
var ConfidenceThreshold = settings.RegisterFloatSetting(
	settings.SystemOnly,
	"autoscale.confidence_threshold",
	"minimum prediction confidence required to trigger scaling (0.0-1.0)",
	0.7,
	settings.FloatInRange(0.0, 1.0),
	settings.WithPublic,
)

// SafetyMargin sets the multiplier applied to predictions for safety.
var SafetyMargin = settings.RegisterFloatSetting(
	settings.SystemOnly,
	"autoscale.safety_margin",
	"multiplier applied to load predictions as a safety buffer",
	1.2,
	settings.FloatInRange(1.0, 3.0),
	settings.WithPublic,
)

// ScaleUpThreshold sets the load ratio that triggers scale up.
var ScaleUpThreshold = settings.RegisterFloatSetting(
	settings.SystemOnly,
	"autoscale.scale_up_threshold",
	"predicted/current load ratio above which to scale up",
	0.8,
	settings.FloatInRange(0.1, 1.0),
	settings.WithPublic,
)

// ScaleDownThreshold sets the load ratio that triggers scale down.
var ScaleDownThreshold = settings.RegisterFloatSetting(
	settings.SystemOnly,
	"autoscale.scale_down_threshold",
	"predicted/current load ratio below which to scale down",
	0.3,
	settings.FloatInRange(0.0, 1.0),
	settings.WithPublic,
)

// MetricsRetention sets how long to retain workload metrics.
var MetricsRetention = settings.RegisterDurationSetting(
	settings.SystemOnly,
	"autoscale.metrics_retention",
	"how long to retain historical workload metrics for pattern analysis",
	30*24*time.Hour, // 30 days
	settings.DurationInRange(1*24*time.Hour, 365*24*time.Hour),
	settings.WithPublic,
)

// CollectionInterval sets how frequently to collect workload metrics.
var CollectionInterval = settings.RegisterDurationSetting(
	settings.SystemOnly,
	"autoscale.collection_interval",
	"how frequently to collect workload metrics",
	1*time.Minute,
	settings.DurationInRange(10*time.Second, 15*time.Minute),
	settings.WithPublic,
)

// PredictionHorizon sets how far into the future to generate predictions.
var PredictionHorizon = settings.RegisterDurationSetting(
	settings.SystemOnly,
	"autoscale.prediction_horizon",
	"how far into the future to generate load predictions",
	24*time.Hour,
	settings.DurationInRange(1*time.Hour, 7*24*time.Hour),
	settings.WithPublic,
)

// GetScalingConfig returns a ScalingConfig populated from cluster settings.
func GetScalingConfig(sv *settings.Values) ScalingConfig {
	return ScalingConfig{
		MinNodes:            int(MinNodes.Get(sv)),
		MaxNodes:            int(MaxNodes.Get(sv)),
		ScaleUpThreshold:    ScaleUpThreshold.Get(sv),
		ScaleDownThreshold:  ScaleDownThreshold.Get(sv),
		LeadTime:            LeadTime.Get(sv),
		Cooldown:            Cooldown.Get(sv),
		ConfidenceThreshold: ConfidenceThreshold.Get(sv),
		SafetyMargin:        SafetyMargin.Get(sv),
	}
}

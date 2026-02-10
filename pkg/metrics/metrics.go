/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"strings"
	"time"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
	klog "k8s.io/klog/v2"
)

const (
	subSystem = "blob_csi_driver"
)

var (
	operationDuration = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Subsystem:      subSystem,
			Name:           "operation_duration_seconds",
			Help:           "Histogram of CSI operation duration in seconds",
			Buckets:        []float64{0.1, 0.2, 0.5, 1, 5, 10, 15, 20, 30, 40, 50, 60, 100, 200, 300},
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"operation", "success"},
	)

	operationDurationWithLabels = metrics.NewHistogramVec(
		&metrics.HistogramOpts{
			Subsystem:      subSystem,
			Name:           "operation_duration_seconds_labeled",
			Help:           "Histogram of CSI operation duration with additional labels",
			Buckets:        []float64{0.1, 0.2, 0.5, 1, 5, 10, 15, 20, 30, 40, 50, 60, 100, 200, 300},
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"operation", "success", "protocol", "storage_account_type", "is_hns_enabled"},
	)

	operationTotal = metrics.NewCounterVec(
		&metrics.CounterOpts{
			Subsystem:      subSystem,
			Name:           "operations_total",
			Help:           "Total number of CSI operations",
			StabilityLevel: metrics.ALPHA,
		},
		[]string{"operation", "success"},
	)
)

func init() {
	legacyregistry.MustRegister(operationDuration)
	legacyregistry.MustRegister(operationDurationWithLabels)
	legacyregistry.MustRegister(operationTotal)
}

// CSIMetricContext represents the context for CSI operation metrics
type CSIMetricContext struct {
	operation     string
	volumeContext map[string]string
	start         time.Time
	labels        map[string]string
	logLevel      int32
	enableLogging bool
}

// NewCSIMetricContext creates a new CSI metric context
func NewCSIMetricContext(operation string) *CSIMetricContext {
	return &CSIMetricContext{
		operation:     operation,
		volumeContext: make(map[string]string),
		start:         time.Now(),
		labels:        make(map[string]string),
		logLevel:      3,
		enableLogging: true,
	}
}

// WithBasicVolumeInfo adds the standard volume-related context to the metric context
func (mc *CSIMetricContext) WithBasicVolumeInfo(resourceGroup, subscriptionID, source string) *CSIMetricContext {
	mc.volumeContext["resource_group"] = strings.ToLower(resourceGroup)
	mc.volumeContext["subscription_id"] = subscriptionID
	mc.volumeContext["source"] = source
	return mc
}

// WithAdditionalVolumeInfo adds additional volume-related context as key-value pairs
// e.g., WithAdditionalVolumeInfo("volume_id", "vol-123")
func (mc *CSIMetricContext) WithAdditionalVolumeInfo(keyValuePairs ...string) *CSIMetricContext {
	if len(keyValuePairs)%2 != 0 {
		return mc
	}
	for i := 0; i < len(keyValuePairs); i += 2 {
		mc.volumeContext[keyValuePairs[i]] = keyValuePairs[i+1]
	}
	return mc
}

// WithLabel adds a label to the metric context
func (mc *CSIMetricContext) WithLabel(key, value string) *CSIMetricContext {
	if mc.labels == nil {
		mc.labels = make(map[string]string)
	}
	mc.labels[key] = value
	return mc
}

// WithLogLevel sets the log level for the metric context
func (mc *CSIMetricContext) WithLogLevel(level int32) *CSIMetricContext {
	mc.logLevel = level
	return mc
}

// WithLogging enables or disables latency logging
func (mc *CSIMetricContext) WithLogging(enabled bool) *CSIMetricContext {
	mc.enableLogging = enabled
	return mc
}

// Observe records the operation result and duration
func (mc *CSIMetricContext) Observe(success bool) {
	duration := time.Since(mc.start).Seconds()
	successStr := "true"
	resultCode := "succeeded"
	if !success {
		successStr = "false"
		resultCode = "failed_" + mc.operation
	}

	// Always record basic metrics
	operationDuration.WithLabelValues(mc.operation, successStr).Observe(duration)
	operationTotal.WithLabelValues(mc.operation, successStr).Inc()

	// Record detailed metrics if labels are present
	if len(mc.labels) > 0 {
		protocol := mc.labels["protocol"]
		storageAccountType := mc.labels["storage_account_type"]
		isHnsEnabled := mc.labels["is_hns_enabled"]

		operationDurationWithLabels.WithLabelValues(
			mc.operation,
			successStr,
			protocol,
			storageAccountType,
			isHnsEnabled,
		).Observe(duration)
	}

	if mc.enableLogging {
		logger := klog.Background().WithName("logLatency")
		keysAndValues := []interface{}{"latency_seconds", duration, "request", subSystem + "_" + mc.operation, "result_code", resultCode}
		for k, v := range mc.volumeContext {
			keysAndValues = append(keysAndValues, k, v)
		}
		logger.V(int(mc.logLevel)).Info("Observed Request Latency", keysAndValues...)
	}
}

// ObserveWithLabels records the operation with provided label pairs
func (mc *CSIMetricContext) ObserveWithLabels(success bool, labelPairs ...string) {
	if len(labelPairs) == 0 || len(labelPairs)%2 != 0 {
		// No label pairs or invalid label pairs, just observe without labels
		mc.Observe(success)
		return
	}

	for i := 0; i < len(labelPairs); i += 2 {
		mc.WithLabel(labelPairs[i], labelPairs[i+1])
	}
	mc.Observe(success)
}

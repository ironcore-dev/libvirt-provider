// SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/workqueue"
)

const (
	WorkQueueSubsystem         = "workqueue"
	DepthKey                   = "depth"
	AddsKey                    = "adds_total"
	QueueLatencyKey            = "queue_duration_seconds"
	WorkDurationKey            = "work_duration_seconds"
	UnfinishedWorkKey          = "unfinished_work_seconds"
	LongestRunningProcessorKey = "longest_running_processor_seconds"
	RetriesKey                 = "retries_total"
)

var (
	ControllerRuntimeReconcileErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_runtime_reconcile_errors_total",
		Help: "Total number of reconciliation errors per controller",
	}, []string{"controller"})

	ControllerRuntimeReconcileDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "controller_runtime_reconcile_duration_seconds",
		Help: "Length of time per reconciliation per controller",
	}, []string{"controller"})

	ControllerRuntimeMaxConccurrentReconciles = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "controller_runtime_max_concurrent_reconciles",
		Help: "Maximum number of concurrent reconciles per controller",
	}, []string{"controller"})

	ControllerRuntimeActiveWorker = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "controller_runtime_active_workers",
		Help: "Number of currently used workers per controller",
	}, []string{"controller"})

	workqueuDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: WorkQueueSubsystem,
		Name:      DepthKey,
		Help:      "Current depth of workqueue",
	}, []string{"name", "controller"})

	workqueueAdds = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: WorkQueueSubsystem,
		Name:      AddsKey,
		Help:      "Total number of adds handled by workqueue",
	}, []string{"name", "controller"})

	workqueueLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: WorkQueueSubsystem,
		Name:      QueueLatencyKey,
		Help:      "How long in seconds an item stays in workqueue before being requested",
		Buckets:   prometheus.ExponentialBuckets(10e-9, 10, 12),
	}, []string{"name", "controller"})

	workqueueDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: WorkQueueSubsystem,
		Name:      WorkDurationKey,
		Help:      "How long in seconds processing an item from workqueue takes.",
		Buckets:   prometheus.ExponentialBuckets(10e-9, 10, 12),
	}, []string{"name", "controller"})

	workqueueUnfinished = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: WorkQueueSubsystem,
		Name:      UnfinishedWorkKey,
		Help: "How many seconds of work has been done that " +
			"is in progress and hasn't been observed by work_duration. Large " +
			"values indicate stuck threads. One can deduce the number of stuck " +
			"threads by observing the rate at which this increases.",
	}, []string{"name", "controller"})

	workqueueLongestRunningProcessor = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: WorkQueueSubsystem,
		Name:      LongestRunningProcessorKey,
		Help: "How many seconds has the longest running " +
			"processor for workqueue been running.",
	}, []string{"name", "controller"})

	workqueueRetries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: WorkQueueSubsystem,
		Name:      RetriesKey,
		Help:      "Total number of retries handled by workqueue",
	}, []string{"name", "controller"})

	OperationDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "operation_duration_seconds",
		Help: "Length of time per operation",
	}, []string{"operation"})

	OperationErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "operation_errors_total",
		Help: "Total number of errors which affect main logic of operation",
	}, []string{"operation"})
)

func init() {
	prometheus.MustRegister(ControllerRuntimeReconcileErrors)
	prometheus.MustRegister(ControllerRuntimeReconcileDuration)
	prometheus.MustRegister(ControllerRuntimeMaxConccurrentReconciles)
	prometheus.MustRegister(ControllerRuntimeActiveWorker)
	prometheus.MustRegister(OperationDuration)
	prometheus.MustRegister(OperationErrors)
	prometheus.MustRegister(workqueuDepth)
	prometheus.MustRegister(workqueueAdds)
	prometheus.MustRegister(workqueueLatency)
	prometheus.MustRegister(workqueueDuration)
	prometheus.MustRegister(workqueueUnfinished)
	prometheus.MustRegister(workqueueLongestRunningProcessor)
	prometheus.MustRegister(workqueueRetries)
	workqueue.SetProvider(WorkqueueMetricsProvider{})
}

type WorkqueueMetricsProvider struct{}

func (WorkqueueMetricsProvider) NewDepthMetric(name string) workqueue.GaugeMetric {
	return workqueuDepth.WithLabelValues(name, name)
}

func (WorkqueueMetricsProvider) NewAddsMetric(name string) workqueue.CounterMetric {
	return workqueueAdds.WithLabelValues(name, name)
}

func (WorkqueueMetricsProvider) NewLatencyMetric(name string) workqueue.HistogramMetric {
	return workqueueLatency.WithLabelValues(name, name)
}

func (WorkqueueMetricsProvider) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	return workqueueDuration.WithLabelValues(name, name)
}

func (WorkqueueMetricsProvider) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return workqueueUnfinished.WithLabelValues(name, name)
}

func (WorkqueueMetricsProvider) NewLongestRunningProcessorSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return workqueueLongestRunningProcessor.WithLabelValues(name, name)
}

func (WorkqueueMetricsProvider) NewRetriesMetric(name string) workqueue.CounterMetric {
	return workqueueRetries.WithLabelValues(name, name)
}

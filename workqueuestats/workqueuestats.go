// Package workqueuestats provides a gostats bridge for the Kubernetes workqueue.
//
// This package is written under the assumption that we're coding
// against the metrics logic as defined here:
// https://github.com/kubernetes/client-go/blob/v0.17.0/util/workqueue/metrics.go
//
// NOTE: gostats traditionally reports timers in microseconds; kube's
// workqueue uses a float representing seconds. Even though gostats is
// wrong to use microseconds, to maintain consistency, we do the
// conversion here.
package workqueuestats

import (
	stats "github.com/lyft/gostats"
	"k8s.io/client-go/util/workqueue"
)

type StatsMetricsProvider struct {
	scope stats.Scope
}

func (s *StatsMetricsProvider) NewDepthMetric(name string) workqueue.GaugeMetric {
	return s.scope.NewGaugeWithTags("depth", map[string]string{"workqueue": name})
}

func (s *StatsMetricsProvider) NewAddsMetric(name string) workqueue.CounterMetric {
	return s.scope.NewCounterWithTags("adds", map[string]string{"workqueue": name})
}

func (s *StatsMetricsProvider) NewRetriesMetric(name string) workqueue.CounterMetric {
	return s.scope.NewCounterWithTags("retries", map[string]string{"workqueue": name})
}

type wrappedTimer struct{ t stats.Timer }

func (t wrappedTimer) Observe(v float64) {
	// assumes v is in seconds - here we convert it to microseconds. see package comment why.
	t.t.AddValue(v * 1000000)
}

type wrappedGauge struct{ t stats.Gauge }

func (t wrappedGauge) Set(v float64) {
	// assumes v is in seconds - here we convert it to microseconds. see package comment why.
	t.t.Set(uint64(v * 1000000))
}

func (s *StatsMetricsProvider) NewLatencyMetric(name string) workqueue.HistogramMetric {
	return wrappedTimer{t: s.scope.NewTimerWithTags("latency", map[string]string{"workqueue": name})}
}

func (s *StatsMetricsProvider) NewWorkDurationMetric(name string) workqueue.HistogramMetric {
	return wrappedTimer{t: s.scope.NewTimerWithTags("workDuration", map[string]string{"workqueue": name})}
}

func (s *StatsMetricsProvider) NewLongestRunningProcessorSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return wrappedGauge{t: s.scope.NewGaugeWithTags("longestRunningProcessor", map[string]string{"workqueue": name})}
}

func (s *StatsMetricsProvider) NewUnfinishedWorkSecondsMetric(name string) workqueue.SettableGaugeMetric {
	return wrappedGauge{t: s.scope.NewGaugeWithTags("unfinishedWork", map[string]string{"workqueue": name})}
}

func New(scope stats.Scope) *StatsMetricsProvider {
	return &StatsMetricsProvider{scope: scope}
}

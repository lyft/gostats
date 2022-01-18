package stats

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type prometheusSink struct {
	counters   map[string]prometheus.Counter
	gauges     map[string]prometheus.Gauge
	histograms map[string]prometheus.Histogram
}

// NewLoggingSink returns a Sink that flushes stats to os.StdErr.
func NewPrometheusSink() Sink {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":9090", nil)
	newSink := &prometheusSink{
		counters:   make(map[string]prometheus.Counter),
		gauges:     make(map[string]prometheus.Gauge),
		histograms: make(map[string]prometheus.Histogram),
	}
	return newSink
}

func (s *prometheusSink) FlushCounter(name string, value uint64) {
	if _, ok := s.counters[name]; !ok {
		s.counters[name] = promauto.NewCounter(prometheus.CounterOpts{Name: name})
	}
	counter := s.counters[name]
	counter.Add(float64(value))
}

func (s *prometheusSink) FlushGauge(name string, value uint64) {
	if _, ok := s.gauges[name]; !ok {
		s.gauges[name] = promauto.NewGauge(prometheus.GaugeOpts{Name: name})
	}
	gauge := s.gauges[name]
	gauge.Set(float64(value))
}

func (s *prometheusSink) FlushTimer(name string, value float64) {
	if _, ok := s.histograms[name]; !ok {
		s.histograms[name] = promauto.NewHistogram(prometheus.HistogramOpts{Name: name})
	}
	histogram := s.histograms[name]
	histogram.Observe(value)
}

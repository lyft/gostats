package stats

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	metrics "github.com/rcrowley/go-metrics"
)

type gostatsRegistry struct {
	r metrics.Registry
}

// Call the given function for each registered metric.
func (r *gostatsRegistry) Each(f func(string, interface{})) {
	r.r.Each(f)
}

// Get the metric by the given name or nil if none is registered.
func (r *gostatsRegistry) Get(name string) interface{} {
	return r.r.Get(name)
}

// GetAll metrics in the Registry.
func (r *gostatsRegistry) GetAll() map[string]map[string]interface{} {
	return r.r.GetAll()
}

// Gets an existing metric or registers the given one.
// The interface can be the metric to register if not found in registry,
// or a function returning the metric for lazy instantiation.
func (r *gostatsRegistry) GetOrRegister(name string, m interface{}) interface{} {
	// TODO: fix.
	return r.r.GetOrRegister(name, m)
}

// Register the given metric under the given name.
func (r *gostatsRegistry) Register(name string, m interface{}) error {
	// TODO: fix.
	return r.r.Register(name, m)
}

// Run all registered healthchecks.
func (r *gostatsRegistry) RunHealthchecks() {
	r.r.RunHealthchecks()
}

// Unregister the metric with the given name.
func (r *gostatsRegistry) Unregister(name string) {
	r.r.Unregister(name)
}

// Unregister all metrics.  (Mostly for testing.)
func (r *gostatsRegistry) UnregisterAll() {
	r.r.UnregisterAll()
}

var _ metrics.Registry = &gostatsRegistry{}

// goMetricsGenerator generates stats to a statStore for a metrics.Registry.
type goMetricsGenerator struct {
	r metrics.Registry
	s *statStore
}

func (g *goMetricsGenerator) GenerateStats() {
	g.r.Each(func(name string, i interface{}) {
		switch metric := i.(type) {
		case metrics.Counter:
			g.s.NewCounter(name).Add(uint64(metric.Count()))
		case metrics.Gauge:
			g.s.NewGauge(name).Set(uint64(metric.Value()))
		case metrics.GaugeFloat64:
			g.s.NewGauge(name).Set(uint64(metric.Value())) // TODO: maybe multiply this by some factor to avoid precision loss.
		case metrics.Histogram:
			// TODO: these aren't supported by statsd. Could possibly try to add some histogram buckets though.
			m := metric.Snapshot()
			g.s.NewGauge(name + ".count").Set(uint64(m.Count()))
			g.s.NewGauge(name + ".min").Set(uint64(m.Min()))
			g.s.NewGauge(name + ".max").Set(uint64(m.Max()))
			g.s.NewGauge(name + ".mean").Set(uint64(m.Mean()))
			g.s.NewGauge(name + ".stddev").Set(uint64(m.StdDev()))
		case metrics.Meter:
			m := metric.Snapshot()
			g.s.NewGauge(name + ".count").Set(uint64(m.Count()))
			g.s.NewGauge(name + ".1m").Set(uint64(m.Rate1()))
			g.s.NewGauge(name + ".5m").Set(uint64(m.Rate5()))
			g.s.NewGauge(name + ".15m").Set(uint64(m.Rate15()))
			g.s.NewGauge(name + ".mean").Set(uint64(m.RateMean()))
		case metrics.Timer:
			m := metric.Snapshot()
			ps := m.Percentiles([]float64{0.5, 0.95, 0.99, 0.999})
			count := m.Count()
			fmt.Fprintf(w, "%s.%s.count %d %d\n", c.Prefix, name, count, now)
			fmt.Fprintf(w, "%s.%s.count_ps %.2f %d\n", c.Prefix, name, float64(count)/flushSeconds, now)
			fmt.Fprintf(w, "%s.%s.min %d %d\n", c.Prefix, name, t.Min()/int64(du), now)
			fmt.Fprintf(w, "%s.%s.max %d %d\n", c.Prefix, name, t.Max()/int64(du), now)
			fmt.Fprintf(w, "%s.%s.mean %.2f %d\n", c.Prefix, name, t.Mean()/du, now)
			fmt.Fprintf(w, "%s.%s.std-dev %.2f %d\n", c.Prefix, name, t.StdDev()/du, now)
			for psIdx, psKey := range c.Percentiles {
				key := strings.Replace(strconv.FormatFloat(psKey*100.0, 'f', -1, 64), ".", "", 1)
				fmt.Fprintf(w, "%s.%s.%s-percentile %.2f %d\n", c.Prefix, name, key, ps[psIdx]/du, now)
			}
			fmt.Fprintf(w, "%s.%s.one-minute %.2f %d\n", c.Prefix, name, t.Rate1(), now)
			fmt.Fprintf(w, "%s.%s.five-minute %.2f %d\n", c.Prefix, name, t.Rate5(), now)
			fmt.Fprintf(w, "%s.%s.fifteen-minute %.2f %d\n", c.Prefix, name, t.Rate15(), now)
			fmt.Fprintf(w, "%s.%s.mean-rate %.2f %d\n", c.Prefix, name, t.RateMean(), now)
		default:
			log.Printf("unable to record metric of type %T\n", i)
		}
	})
}

var _ StatGenerator = &goMetricsGenerator{}

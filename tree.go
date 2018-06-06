package stats

import (
	"sync"
	"time"
)

type registry struct {
	tree node
	vals sync.Map // map[*node]*values
}

func (r *registry) getValue(n *node) *values {
	return nil
}

func (r *registry) Flush() {
	panic("not implemented")
}

func (r *registry) Start(*time.Ticker) {
	panic("not implemented")
}

func (r *registry) AddStatGenerator(StatGenerator) {
	panic("not implemented")
}

func (r *registry) Scope(name string) Scope {
	panic("not implemented")
}

func (r *registry) Store() Store {
	panic("not implemented")
}

func (r *registry) NewCounter(name string) Counter {
	panic("not implemented")
}

func (r *registry) NewCounterWithTags(name string, tags map[string]string) Counter {
	panic("not implemented")
}

func (r *registry) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	panic("not implemented")
}

func (r *registry) NewGauge(name string) Gauge {
	panic("not implemented")
}

func (r *registry) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	panic("not implemented")
}

func (r *registry) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	panic("not implemented")
}

func (r *registry) NewTimer(name string) Timer {
	panic("not implemented")
}

func (r *registry) NewTimerWithTags(name string, tags map[string]string) Timer {
	panic("not implemented")
}

func (r *registry) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	panic("not implemented")
}

type values struct {
	counter
	gauge
	timer
}

func newRegistry() *registry {
	r := &registry{
		vals: make(map[*node]*values),
	}
	r.tree.registry = r
	return r
}

type node struct {
	registry *registry
	children sync.Map
}

func (n *node) Scope(name string) Scope {
	return n.child(name)
}

func (n *node) Store() Store {
	return n.registry
}

func (n *node) NewCounter(name string) Counter {
	return n.registry.getValue(n.child(name)).counter
}

func (n *node) NewCounterWithTags(name string, tags map[string]string) Counter {
	panic("not implemented")
}

func (n *node) NewPerInstanceCounter(name string, tags map[string]string) Counter {
	panic("not implemented")
}

func (n *node) NewGauge(name string) Gauge {
	panic("not implemented")
}

func (n *node) NewGaugeWithTags(name string, tags map[string]string) Gauge {
	panic("not implemented")
}

func (n *node) NewPerInstanceGauge(name string, tags map[string]string) Gauge {
	panic("not implemented")
}

func (n *node) NewTimer(name string) Timer {
	panic("not implemented")
}

func (n *node) NewTimerWithTags(name string, tags map[string]string) Timer {
	panic("not implemented")
}

func (n *node) NewPerInstanceTimer(name string, tags map[string]string) Timer {
	panic("not implemented")
}

func (n *node) child(s string) *node {
	if val, ok := n.children.Load(s); ok {
		return val.(*node)
	}
	ret := &node{
		registry: n.registry,
	}
	n.children.Store(s, ret)
	return ret
}

func (n *node) Find(path ...string) *node {
	curr := n
	for i := 0; i < len(path); i++ {
		curr = curr.child(path[i])
	}
	return curr
}

var _ Scope = (*node)(nil)

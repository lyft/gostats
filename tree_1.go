package stats

import "sync"

type tree struct {
	mu   sync.RWMutex
	root node
}

type node struct {
	counter  *counter
	gauge    *gauge
	timer    *timer
	children map[string]*node
}

func (root *node) findOrCreate(path ...string) *node {
	curr := root
	for i := 0; i < len(path); i++ { // 0-alloc
		if curr.children == nil {
			curr.children = make(map[string]*node)
		}
		if curr.children[path[i]] == nil {
			curr.children[path[i]] = new(node)
		}
		curr = curr.children[path[i]]
	}
	return curr
}

func (root *node) GetCounter(path ...string) Counter {
	curr := root.findOrCreate(path...)
	if curr.counter != nil {
		curr.counter = new(counter)
	}
	ret := curr.counter
	return ret
}

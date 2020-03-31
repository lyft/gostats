package main

import stats "github.com/lyft/gostats"

// Test that the stats of a fast exiting program (such as one that immediately
// errors on startup) can send stats.
func main() {
	store := stats.NewDefaultStore()
	store.Scope("test.fast.exit").NewCounter("counter").Inc()
	store.Flush()
}

//go:build travis
// +build travis

package stats

import "testing"

// Travis CI is really slow - so don't run tests in parallel.
func Parallel(t *testing.T) { /* no-op */ }

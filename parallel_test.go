//go:build !travis
// +build !travis

package stats

import "testing"

func Parallel(t *testing.T) { t.Parallel() }

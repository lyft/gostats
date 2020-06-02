// +build tools

package stats

// Tools used at build time that "go mod tidy" shouldn't remove.
import (
	_ "golang.org/x/lint/golint"
)

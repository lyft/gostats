// +build tools

package stats

import (
	_ "golang.org/x/lint"
	_ "golang.org/x/tools/go/analysis/passes/findcall"
	_ "golang.org/x/tools/go/analysis/passes/ifaceassert"
	_ "golang.org/x/tools/go/analysis/passes/lostcancel"
	_ "golang.org/x/tools/go/analysis/passes/nilness"
	_ "golang.org/x/tools/go/analysis/passes/shadow"
	_ "golang.org/x/tools/go/analysis/passes/stringintconv"
	_ "golang.org/x/tools/go/analysis/passes/unmarshal"
)

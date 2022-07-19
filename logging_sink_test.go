package stats

import (
	"os"
)

func Example_flushCounter() {
	l := &loggingSink{writer: os.Stdout, now: foreverNow}
	l.FlushCounter("counterName", 420)
	// Output:
	// {"level":"info","ts":1640995200.000000,"logger":"gostats.loggingsink","msg":"flushing counter","json":{"name":"counterName","type":"counter","value":"420.000000"}}
}

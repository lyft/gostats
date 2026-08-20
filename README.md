# Gostats [![GoDoc](https://godoc.org/github.com/lyft/gostats?status.svg)](https://godoc.org/github.com/lyft/gostats) [![Build Status](https://github.com/lyft/gostats/actions/workflows/actions.yml/badge.svg?branch=master)](https://github.com/lyft/gostats/actions/workflows/actions.yml)

`gostats` is a Go metrics library with support for Counters, Gauges, and Timers.

## Installation

```sh
go get github.com/lyft/gostats
```

## Building & Testing

```sh
go test ./...
```

## Usage

In order to start using `gostats`, import it into your project with:

```go
import "github.com/lyft/gostats"
```


## Bounding memory from high-cardinality tags

By default `gostats` keeps every unique counter and timer name it has ever seen for the life of the
process, even after a value stops changing. A service that tags a Counter or Timer with something
that varies per request - a user ID, a request ID, anything effectively unbounded - will grow
memory without limit, because nothing ever removes an entry.

Set `GOSTATS_PRUNE_IDLE_SECONDS` to prune counters and timers that have gone that many seconds
without being written to. It is unset (disabled) by default, so existing behavior does not change
unless you opt in.

```sh
export GOSTATS_PRUNE_IDLE_SECONDS=60
```

A pruned entry is not gone for good. If your code holds a `Counter` or `Timer` in a struct field -
the common pattern - and writes to it again later, that write reattaches it to the store and it
resumes reporting correctly, including the value from the write that triggered the reattachment.
Nothing is lost. Gauges are never pruned: a gauge holds state a later read may depend on, and
pruning one could make it stop reporting.

Pick a value comfortably longer than the slowest-firing counter or timer you still care about. One
that legitimately fires less often than `GOSTATS_PRUNE_IDLE_SECONDS` will still work correctly, but
each time it goes idle that long it gets pruned and then has to reattach on its next write, delaying
that one report by up to a flush interval.

The seconds value is converted to a number of flushes using your configured flush interval
(`GOSTATS_FLUSH_INTERVAL_SECONDS`, 5 by default), rounded up. If you drive `Start` with your own
ticker at a different period, pruning follows that many of your own flushes, not
`GOSTATS_PRUNE_IDLE_SECONDS` of wall-clock time.

Once enabled, two metrics are emitted (silent otherwise, so turning pruning on is what turns the
visibility on too):

* `gostats.tracked`, tagged `type=counter|gauge|timer` - how many of each are currently held.
  Alert on this climbing without leveling off: that means something is generating unique tag
  values faster than they go idle.
* `gostats.pruned`, tagged `type=counter|timer` - how many were removed on a given flush. A
  sustained high rate means high-cardinality tags are actively being generated and pruned away -
  the leak is contained, but the tag usage generating it is still worth fixing at the source.

## Mocking

A thread-safe mock sink is provided by the [gostats/mock](https://github.com/lyft/gostats/blob/mock-sink/mock/sink.go) package.  The mock sink also provides methods that are useful for testing (as demonstrated below).
```go
package mock_test

import (
	"testing"

	"github.com/lyft/gostats"
	"github.com/lyft/gostats/mock"
)

type Config struct {
	Stats stats.Store
}

func TestMockExample(t *testing.T) {
	sink := mock.NewSink()
	conf := Config{
		Stats: stats.NewStore(sink, false),
	}
	conf.Stats.NewCounter("name").Inc()
	conf.Stats.Flush()
	sink.AssertCounterEquals(t, "name", 1)
}
```

If you do not need to assert on the contents of the sink the below example can be used to quickly create a thread-safe `stats.Scope`:
```go
package config

import (
	"github.com/lyft/gostats"
	"github.com/lyft/gostats/mock"
)

type Config struct {
	Stats stats.Store
}

func NewConfig() *Config {
	return &Config{
		Stats: stats.NewDefaultStore(),
	}
}

func NewMockConfig() *Config {
	sink := mock.NewSink()
	return &Config{
		Stats: stats.NewStore(sink, false),
	}
}
```

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

By default, `gostats` never forgets a counter or timer name once it sees one, even after the value
stops changing. Don't tag one with something that varies per request - a user ID, a request ID,
anything effectively unbounded: that grows memory without limit. `GOSTATS_PRUNE_IDLE_SECONDS` bounds
the damage if that happens anyway. See [docs/idle-pruning.md](docs/idle-pruning.md) for how it
works, how to pick a value, and the metrics it emits.

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

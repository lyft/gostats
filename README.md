<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Gostats](#gostats)
  - [Overview](#overview)
  - [Installation](#installation)
  - [Building](#building)
  - [Usage](#usage)
    - [Settings](#settings)
    - [Store](#store)
      - [StatGenerators](#statgenerators)
      - [Flushing](#flushing)
    - [Scopes](#scopes)
    - [Types of Stats](#types-of-stats)
      - [Counters](#counters)
      - [Gauges](#gauges)
      - [Timers](#timers)
    - [Stats with Tags](#stats-with-tags)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Gostats

## Overview

Gostats is a Golang statistics library with support for Counters, Gauges, and Timers.

## Installation

```
go get github.com/lyft/gostats
```

## Building

```
make bootstrap && make compile-test
```

## Usage
 
In order to start using gostats, import it to your project with:

```Go
import "github.com/lyft/gostats"
```

### Settings

The gostats library uses environment variables to retrieve [settings](https://github.com/lyft/gostats/blob/master/settings.go).
The following settings are currently supported:


|Environment Variable|Type|Default|Description|
|---|---|---|---|
|`USE_STATSD`|`bool`|`true`|Are you using statsd as a backing store|
|`STATSD_HOST`|`string`|`localhost`|Address of the host running statsd|
|`STATSD_PORT`|`int`|`8125`|What port is statsd listening on|
|`GOSTATS_FLUSH_INTERVAL_SECONDS`|`int`|`5`|How often is the a store created by `NewDefaultStore()` going to flush stats|

### Store

There are two options when creating a new store: 

```Go
// create a store backed by a tcp_sink to statsd
s := stats.NewDefaultStore()
// create a store with a user provided Sink
s := stats.NewStore(sink, true)
```

Currently that only backing store supported is statsd via the [tcp_sink](https://github.com/lyft/gostats/blob/master/tcp_sink.go).
However, implementing other [Sinks](https://github.com/lyft/gostats/blob/master/sink.go) should be simple.

A store holds Counters, Gauges, and Timers. You can add Unscoped Counters, Gauges, and Timers to the store
with:

```Go
s := stats.NewDefaultStore()
c := s.New[Counter|Gauge|Timer]("name")
```

#### StatGenerators

StatGenerators can be used to programatically generate stats. StatGenerators implement the following interface:

```Go
type StatGenerator interface {
	GenerateStats()
}
```

StatGenerators are added to a store via the `AddStatGenerator(StatGenerator)` method. You can find an example of a 
StatGenerator [here](https://github.com/lyft/gostats/blob/master/runtime.go).

#### Flushing

To flush the store at a regular interval call the `Start(*time.Ticker)` method on it.

The store will flush either at the regular interval, or whenever `Flush()` is called. Whenever the store is flushed, 
the store will Call `GenerateStats()`on all of its stat generators, and flush all the Counters and Gauges registered with it.

### Scopes

Statistics can be namespaced by creating a Scope:

```Go
store := stats.NewDefaultStore()
scope := stats.Scope("service")
// the following counter will be emitted at the stats tree rooted at `service`.
c := scope.NewCounter("success")
```

Additionally you can create subscopes:

```Go
store := stats.NewDefaultStore()
scope := stats.Scope("service")
networkScope := scope.Scope("network")
// the following counter will be emitted at the stats tree rooted at `service.network`.
c := networkScope.NewCounter("requests")
```

### Types of Stats

#### Counters

Counters are an always incrementing stat. Counters implement the following interface:

```Go
type Counter interface {
	Add(uint64)     // increment the counter by the argument's value.
	Inc()           // increment the counter by 1.
	Set(uint64)     // sets an internal counter value which will be written in the next flush. Its use is discouraged as it may break the counter's "always incrementing" semantics.
	String() string // return the current value of the counter as a string.
	Value() uint64  // return the current value of the counter as a uint64.
}
```

Counters are added to a store, or a scope by using the `NewCounter(name string)` method.

#### Gauges

A Gauge can both increment and decrement. Gauges implement the following interface:

```Go
type Gauge interface {
	Add(uint64)      // increment the gauge by the argument's value.
	Sub(uint64)      // decrement the gauge by the argument's value .
	Inc()            // increment the gauge by 1.
	Dec()            // decrement the gauge by 1.
	Set(uint64)      // set the gauge to the argument's value.
	String() string  // return the current value of the gauge as a string.
	Value() uint64   // return the current value of the gauge as a uint64.
}
```

Gauges are added to a store, or a scope by using the `NewCounter(name string)` method.

#### Timers

Timers can be used to flush timing statistics. Timers implement the following interface:

```Go
type Timer interface {
	AddValue(float64)        // flushes the timer with the argument's value.
	AllocateSpan() Timespan  // allocates a Timespan.
}
```

Timers are added to a store, or a scope by using the `NewTimer(name string)` method.

Timespans can be used to measure spans of time. They measure time from the time they are allocated by a Timer with `AllocateSpan()`,
until they call `Complete()`. When `Complete()` is called the timespan is flushed. A Timespan can be flushed at function
return by calling `Complete()` with golang's `defer` statement.

### Stats with Tags

Counters, Gauges and Timers can have tags in addition to their names. All three have methods 
`New[Counter|Gauge|Timer]WithTags(name string, tags map[string]string)`.
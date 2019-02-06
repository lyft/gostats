# Gostats [![GoDoc](https://godoc.org/github.com/lyft/gostats?status.svg)](https://godoc.org/github.com/lyft/gostats) [![Build Status](https://travis-ci.org/lyft/gostats.svg)](https://travis-ci.org/lyft/gostats)

`gostats` is a Go metrics library with support for Counters, Gauges, and Timers.

## Installation

```sh
go get github.com/lyft/gostats
```

## Building & Testing

```sh
make install 
make test
```

## Usage

In order to start using `gostats`, import it into your project with:

```go
import "github.com/lyft/gostats"
```

## Timers
It is not obvious but due to a but all gostats timers output timespans in *microseconds*, not milliseconds as the stat string itself suggests.
